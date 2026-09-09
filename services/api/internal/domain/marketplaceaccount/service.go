// Package marketplaceaccount owns business/trading identities and marketplace seller-account configuration.
// It deliberately does not own workstation identity, printer credentials, Product Master records, or Inventory.
package marketplaceaccount

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/platform/audit"
	"github.com/commerceops/commerceops/services/api/internal/platform/auth"
	"github.com/commerceops/commerceops/services/api/internal/platform/authorization"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidInput = errors.New("invalid marketplace account input")
	ErrNotFound     = errors.New("business identity or marketplace account not found")
	ErrConflict     = errors.New("marketplace account conflict")
	idPattern       = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89aAbB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
	keyPattern      = regexp.MustCompile(`^[a-z][a-z0-9_]{0,79}$`)
)

type IdentityInput struct {
	DisplayName string  `json:"display_name"`
	LegalName   *string `json:"legal_name"`
	Status      string  `json:"status"`
}

type Identity struct {
	ID string `json:"id"`
	IdentityInput
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AccountInput struct {
	MarketplaceKey     string  `json:"marketplace_key"`
	BusinessIdentityID string  `json:"business_identity_id"`
	InternalKey        string  `json:"internal_key"`
	DisplayName        string  `json:"display_name"`
	ExternalSellerID   *string `json:"external_seller_id"`
	IntegrationMode    string  `json:"integration_mode"`
	Status             string  `json:"status"`
}

type Account struct {
	ID string `json:"id"`
	AccountInput
	IdentityName string    `json:"identity_name"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Service struct {
	db    *pgxpool.Pool
	authz *authorization.Service
	audit audit.Recorder
}

func NewService(db *pgxpool.Pool, authz *authorization.Service) *Service {
	return &Service{db: db, authz: authz}
}

func (s *Service) Identities(ctx context.Context, p auth.Principal) ([]Identity, error) {
	if err := s.authz.RequirePermission(ctx, p, "marketplace_accounts.view"); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT id,display_name,legal_name,status,created_at,updated_at FROM business_identities WHERE company_id=$1 ORDER BY display_name,id`, p.CompanyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Identity{}
	for rows.Next() {
		var item Identity
		if err = rows.Scan(&item.ID, &item.DisplayName, &item.LegalName, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Service) SaveIdentity(ctx context.Context, p auth.Principal, id string, input IdentityInput) (Identity, error) {
	if err := s.authz.RequirePermission(ctx, p, "marketplace_accounts.manage"); err != nil {
		return Identity{}, err
	}
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if !validIdentity(input) || (id != "" && !idPattern.MatchString(id)) {
		return Identity{}, ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Identity{}, err
	}
	defer tx.Rollback(ctx)
	var before *Identity
	if id == "" {
		err = tx.QueryRow(ctx, `INSERT INTO business_identities(company_id,display_name,legal_name,status) VALUES($1,$2,$3,$4) RETURNING id`, p.CompanyID, input.DisplayName, optional(input.LegalName), input.Status).Scan(&id)
	} else {
		previous, readErr := identityByID(ctx, tx, p.CompanyID, id, true)
		if readErr != nil {
			return Identity{}, readErr
		}
		before = &previous
		_, err = tx.Exec(ctx, `UPDATE business_identities SET display_name=$3,legal_name=$4,status=$5,updated_at=now() WHERE company_id=$1 AND id=$2`, p.CompanyID, id, input.DisplayName, optional(input.LegalName), input.Status)
	}
	if err != nil {
		return Identity{}, mapError(err)
	}
	result, err := identityByID(ctx, tx, p.CompanyID, id, false)
	if err != nil {
		return Identity{}, err
	}
	action := "business_identity.created"
	if before != nil {
		action = "business_identity.updated"
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, action, "business_identity", id, map[string]any{"before": before, "after": result}); err != nil {
		return Identity{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Identity{}, err
	}
	return result, nil
}

func (s *Service) Accounts(ctx context.Context, p auth.Principal, marketplace string) ([]Account, error) {
	if err := s.authz.RequirePermission(ctx, p, "marketplace_accounts.view"); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, accountSelect+` WHERE a.company_id=$1 AND ($2='' OR a.marketplace_key=$2) ORDER BY a.marketplace_key,a.display_name,a.id`, p.CompanyID, marketplace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Account{}
	for rows.Next() {
		item, scanErr := scanAccount(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Service) SaveAccount(ctx context.Context, p auth.Principal, id string, input AccountInput) (Account, error) {
	if err := s.authz.RequirePermission(ctx, p, "marketplace_accounts.manage"); err != nil {
		return Account{}, err
	}
	input.MarketplaceKey = strings.TrimSpace(input.MarketplaceKey)
	input.InternalKey = strings.TrimSpace(input.InternalKey)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if !validAccount(input) || (id != "" && !idPattern.MatchString(id)) {
		return Account{}, ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Account{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = identityByID(ctx, tx, p.CompanyID, input.BusinessIdentityID, true); err != nil {
		return Account{}, err
	}
	var before *Account
	if id == "" {
		err = tx.QueryRow(ctx, `INSERT INTO marketplace_accounts(company_id,marketplace_key,business_identity_id,internal_key,display_name,external_seller_id,integration_mode,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, p.CompanyID, input.MarketplaceKey, input.BusinessIdentityID, input.InternalKey, input.DisplayName, optional(input.ExternalSellerID), input.IntegrationMode, input.Status).Scan(&id)
	} else {
		previous, readErr := accountByID(ctx, tx, p.CompanyID, id, true)
		if readErr != nil {
			return Account{}, readErr
		}
		before = &previous
		if previous.MarketplaceKey != input.MarketplaceKey {
			return Account{}, ErrConflict
		}
		_, err = tx.Exec(ctx, `UPDATE marketplace_accounts SET business_identity_id=$3,internal_key=$4,display_name=$5,external_seller_id=$6,integration_mode=$7,status=$8,updated_at=now() WHERE company_id=$1 AND id=$2`, p.CompanyID, id, input.BusinessIdentityID, input.InternalKey, input.DisplayName, optional(input.ExternalSellerID), input.IntegrationMode, input.Status)
	}
	if err != nil {
		return Account{}, mapError(err)
	}
	result, err := accountByID(ctx, tx, p.CompanyID, id, false)
	if err != nil {
		return Account{}, err
	}
	action := "marketplace_account.created"
	if before != nil {
		action = "marketplace_account.updated"
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, action, "marketplace_account", id, map[string]any{"before": before, "after": result}); err != nil {
		return Account{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Account{}, err
	}
	return result, nil
}

func validIdentity(input IdentityInput) bool {
	return input.DisplayName != "" && len(input.DisplayName) <= 120 && validStatus(input.Status) && validOptional(input.LegalName, 250)
}
func validAccount(input AccountInput) bool {
	return input.MarketplaceKey != "" && idPattern.MatchString(input.BusinessIdentityID) && keyPattern.MatchString(input.InternalKey) && input.DisplayName != "" && len(input.DisplayName) <= 120 && validOptional(input.ExternalSellerID, 200) && validStatus(input.Status) && validMode(input.IntegrationMode)
}
func validStatus(value string) bool {
	return value == "active" || value == "dormant" || value == "inactive"
}
func validMode(value string) bool {
	return value == "manual_upload" || value == "api" || value == "file_watch" || value == "print_capture" || value == "partner_connector"
}
func validOptional(value *string, maximum int) bool {
	return value == nil || (strings.TrimSpace(*value) == *value && *value != "" && len(*value) <= maximum)
}
func optional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}
func mapError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	if errors.As(err, &pgErr) && (pgErr.Code == "23503" || pgErr.Code == "23514") {
		return ErrInvalidInput
	}
	return err
}

type rowScanner interface{ Scan(...any) error }
type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func identityByID(ctx context.Context, q queryer, companyID, id string, lock bool) (Identity, error) {
	var item Identity
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	err := q.QueryRow(ctx, `SELECT id,display_name,legal_name,status,created_at,updated_at FROM business_identities WHERE company_id=$1 AND id=$2`+suffix, companyID, id).Scan(&item.ID, &item.DisplayName, &item.LegalName, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	return item, mapError(err)
}

const accountSelect = `SELECT a.id,a.marketplace_key,a.business_identity_id,a.internal_key,a.display_name,a.external_seller_id,a.integration_mode,a.status,b.display_name,a.created_at,a.updated_at FROM marketplace_accounts a JOIN business_identities b ON b.company_id=a.company_id AND b.id=a.business_identity_id`

func scanAccount(row rowScanner) (Account, error) {
	var item Account
	err := row.Scan(&item.ID, &item.MarketplaceKey, &item.BusinessIdentityID, &item.InternalKey, &item.DisplayName, &item.ExternalSellerID, &item.IntegrationMode, &item.Status, &item.IdentityName, &item.CreatedAt, &item.UpdatedAt)
	return item, mapError(err)
}
func accountByID(ctx context.Context, q queryer, companyID, id string, lock bool) (Account, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE OF a"
	}
	return scanAccount(q.QueryRow(ctx, accountSelect+` WHERE a.company_id=$1 AND a.id=$2`+suffix, companyID, id))
}
