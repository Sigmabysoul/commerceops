// Package marketplaceaccount owns seller/trading identity and account lifecycle.
// Machine identity, credentials, Product Master and Inventory remain separate.
package marketplaceaccount

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/audit"
	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidInput = errors.New("invalid marketplace account input")
	ErrNotFound     = errors.New("marketplace account or identity not found")
	ErrConflict     = errors.New("marketplace account conflict")
	ErrUnavailable  = errors.New("marketplace account is not active or is unassigned legacy history")
	uuidRE          = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89aAbB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
	keyRE           = regexp.MustCompile(`^[a-z][a-z0-9_]{0,79}$`)
)

type IdentityInput struct {
	DisplayName string  `json:"display_name"`
	LegalName   *string `json:"legal_name"`
	Status      string  `json:"status"`
}
type Identity struct {
	IdentityInput
	ID               string    `json:"id"`
	LegacyUnassigned bool      `json:"legacy_unassigned"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
type AccountInput struct {
	MarketplaceKey     string            `json:"marketplace_key"`
	BusinessIdentityID string            `json:"business_identity_id"`
	InternalKey        string            `json:"internal_key"`
	DisplayName        string            `json:"display_name"`
	ExternalSellerID   *string           `json:"external_seller_id"`
	FulfillmentModel   *string           `json:"fulfillment_model"`
	IntegrationMode    string            `json:"integration_mode"`
	Status             string            `json:"status"`
	Metadata           map[string]string `json:"metadata"`
}
type Account struct {
	AccountInput
	ID               string    `json:"id"`
	IdentityName     string    `json:"identity_name"`
	LegacyUnassigned bool      `json:"legacy_unassigned"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
type Service struct {
	db    *pgxpool.Pool
	authz *authorization.Service
	audit audit.Recorder
}

func NewService(db *pgxpool.Pool, a *authorization.Service) *Service {
	return &Service{db: db, authz: a}
}
func (s *Service) require(ctx context.Context, p auth.Principal, manage bool) error {
	key := "marketplace_accounts.view"
	if manage {
		key = "marketplace_accounts.manage"
	}
	return s.authz.RequirePermission(ctx, p, key)
}
func validStatus(v string) bool { return v == "active" || v == "dormant" || v == "inactive" }
func optional(v *string, max int) bool {
	return v == nil || strings.TrimSpace(*v) == *v && len(*v) > 0 && len(*v) <= max
}
func mapError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var p *pgconn.PgError
	if errors.As(err, &p) {
		switch p.Code {
		case "23505":
			return ErrConflict
		case "23503", "23514", "22P02":
			return ErrInvalidInput
		}
	}
	return err
}

type querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Validate uses explicit account identity, never a marketplace-only fallback.
// active is required for new ingestion; historical reads may include dormancy.
func Validate(ctx context.Context, q querier, company, id, marketplace string, active bool) error {
	if !uuidRE.MatchString(id) {
		return ErrInvalidInput
	}
	var status, identityStatus string
	var legacy bool
	err := q.QueryRow(ctx, `SELECT a.status,b.status,a.legacy_unassigned FROM marketplace_accounts a JOIN business_identities b ON b.company_id=a.company_id AND b.id=a.business_identity_id WHERE a.company_id=$1 AND a.id=$2 AND ($3='' OR a.marketplace_key=$3)`, company, id, marketplace).Scan(&status, &identityStatus, &legacy)
	if err != nil {
		return mapError(err)
	}
	if legacy || active && (status != "active" || identityStatus != "active") {
		return ErrUnavailable
	}
	return nil
}

const accountSelect = `SELECT a.id,a.marketplace_key,a.business_identity_id,a.internal_key,a.display_name,a.external_seller_id,a.fulfillment_model,a.integration_mode,a.status,a.metadata,a.legacy_unassigned,a.created_at,a.updated_at,b.display_name FROM marketplace_accounts a JOIN business_identities b ON b.company_id=a.company_id AND b.id=a.business_identity_id`

func scanAccount(row interface{ Scan(...any) error }) (Account, error) {
	var a Account
	var raw []byte
	err := row.Scan(&a.ID, &a.MarketplaceKey, &a.BusinessIdentityID, &a.InternalKey, &a.DisplayName, &a.ExternalSellerID, &a.FulfillmentModel, &a.IntegrationMode, &a.Status, &raw, &a.LegacyUnassigned, &a.CreatedAt, &a.UpdatedAt, &a.IdentityName)
	if err != nil {
		return a, mapError(err)
	}
	err = json.Unmarshal(raw, &a.Metadata)
	return a, err
}
func (s *Service) Accounts(ctx context.Context, p auth.Principal, marketplace, status string) ([]Account, error) {
	if err := s.require(ctx, p, false); err != nil {
		return nil, err
	}
	if status != "" && !validStatus(status) {
		return nil, ErrInvalidInput
	}
	rows, err := s.db.Query(ctx, accountSelect+` WHERE a.company_id=$1 AND ($2='' OR a.marketplace_key=$2) AND ($3='' OR a.status=$3) ORDER BY a.marketplace_key,a.display_name,a.id`, p.CompanyID, marketplace, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Account{}
	for rows.Next() {
		a, e := scanAccount(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (s *Service) Account(ctx context.Context, p auth.Principal, id string) (Account, error) {
	if err := s.require(ctx, p, false); err != nil {
		return Account{}, err
	}
	if !uuidRE.MatchString(id) {
		return Account{}, ErrInvalidInput
	}
	return scanAccount(s.db.QueryRow(ctx, accountSelect+` WHERE a.company_id=$1 AND a.id=$2`, p.CompanyID, id))
}
func (s *Service) SaveAccount(ctx context.Context, p auth.Principal, id string, in AccountInput) (Account, error) {
	if err := s.require(ctx, p, true); err != nil {
		return Account{}, err
	}
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if in.Metadata == nil {
		in.Metadata = map[string]string{}
	}
	validMode := map[string]bool{"manual_upload": true, "api": true, "file_watch": true, "print_capture": true, "partner_connector": true}
	if id != "" && !uuidRE.MatchString(id) || !uuidRE.MatchString(in.BusinessIdentityID) || !keyRE.MatchString(in.InternalKey) || in.DisplayName == "" || len(in.DisplayName) > 120 || !validStatus(in.Status) || !validMode[in.IntegrationMode] || !optional(in.ExternalSellerID, 200) || !optional(in.FulfillmentModel, 80) {
		return Account{}, ErrInvalidInput
	}
	for k, v := range in.Metadata {
		if k != "region" && k != "notes" || len(v) > 2000 {
			return Account{}, ErrInvalidInput
		}
	}
	raw, err := json.Marshal(in.Metadata)
	if err != nil || len(raw) > 4096 {
		return Account{}, ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Account{}, err
	}
	defer tx.Rollback(ctx)
	var legacy bool
	if err = tx.QueryRow(ctx, `SELECT legacy_unassigned FROM business_identities WHERE company_id=$1 AND id=$2 FOR SHARE`, p.CompanyID, in.BusinessIdentityID).Scan(&legacy); err != nil {
		return Account{}, mapError(err)
	}
	if legacy {
		return Account{}, ErrUnavailable
	}
	var before *Account
	if id == "" {
		err = tx.QueryRow(ctx, `INSERT INTO marketplace_accounts(company_id,marketplace_key,business_identity_id,internal_key,display_name,external_seller_id,fulfillment_model,integration_mode,status,metadata) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`, p.CompanyID, in.MarketplaceKey, in.BusinessIdentityID, in.InternalKey, in.DisplayName, in.ExternalSellerID, in.FulfillmentModel, in.IntegrationMode, in.Status, raw).Scan(&id)
	} else {
		prior, e := scanAccount(tx.QueryRow(ctx, accountSelect+` WHERE a.company_id=$1 AND a.id=$2 FOR UPDATE OF a`, p.CompanyID, id))
		if e != nil {
			return Account{}, e
		}
		if prior.LegacyUnassigned || prior.MarketplaceKey != in.MarketplaceKey {
			return Account{}, ErrConflict
		}
		before = &prior
		_, err = tx.Exec(ctx, `UPDATE marketplace_accounts SET business_identity_id=$3,internal_key=$4,display_name=$5,external_seller_id=$6,fulfillment_model=$7,integration_mode=$8,status=$9,metadata=$10,updated_at=now() WHERE company_id=$1 AND id=$2`, p.CompanyID, id, in.BusinessIdentityID, in.InternalKey, in.DisplayName, in.ExternalSellerID, in.FulfillmentModel, in.IntegrationMode, in.Status, raw)
	}
	if err != nil {
		return Account{}, mapError(err)
	}
	result, err := scanAccount(tx.QueryRow(ctx, accountSelect+` WHERE a.company_id=$1 AND a.id=$2`, p.CompanyID, id))
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
	return result, tx.Commit(ctx)
}
func scanIdentity(row interface{ Scan(...any) error }) (Identity, error) {
	var i Identity
	err := row.Scan(&i.ID, &i.DisplayName, &i.LegalName, &i.Status, &i.LegacyUnassigned, &i.CreatedAt, &i.UpdatedAt)
	return i, mapError(err)
}

const identitySelect = `SELECT id,display_name,legal_name,status,legacy_unassigned,created_at,updated_at FROM business_identities`

func (s *Service) Identities(ctx context.Context, p auth.Principal) ([]Identity, error) {
	if err := s.require(ctx, p, false); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, identitySelect+` WHERE company_id=$1 ORDER BY display_name,id`, p.CompanyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Identity{}
	for rows.Next() {
		i, e := scanIdentity(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
func (s *Service) SaveIdentity(ctx context.Context, p auth.Principal, id string, in IdentityInput) (Identity, error) {
	if err := s.require(ctx, p, true); err != nil {
		return Identity{}, err
	}
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if id != "" && !uuidRE.MatchString(id) || in.DisplayName == "" || len(in.DisplayName) > 120 || !optional(in.LegalName, 250) || !validStatus(in.Status) {
		return Identity{}, ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Identity{}, err
	}
	defer tx.Rollback(ctx)
	var before *Identity
	if id == "" {
		err = tx.QueryRow(ctx, `INSERT INTO business_identities(company_id,display_name,legal_name,status) VALUES($1,$2,$3,$4) RETURNING id`, p.CompanyID, in.DisplayName, in.LegalName, in.Status).Scan(&id)
	} else {
		prior, e := scanIdentity(tx.QueryRow(ctx, identitySelect+` WHERE company_id=$1 AND id=$2 FOR UPDATE`, p.CompanyID, id))
		if e != nil {
			return Identity{}, e
		}
		if prior.LegacyUnassigned {
			return Identity{}, ErrConflict
		}
		before = &prior
		_, err = tx.Exec(ctx, `UPDATE business_identities SET display_name=$3,legal_name=$4,status=$5,updated_at=now() WHERE company_id=$1 AND id=$2`, p.CompanyID, id, in.DisplayName, in.LegalName, in.Status)
	}
	if err != nil {
		return Identity{}, mapError(err)
	}
	result, err := scanIdentity(tx.QueryRow(ctx, identitySelect+` WHERE company_id=$1 AND id=$2`, p.CompanyID, id))
	if err != nil {
		return Identity{}, err
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "business_identity.saved", "business_identity", id, map[string]any{"before": before, "after": result}); err != nil {
		return Identity{}, err
	}
	return result, tx.Commit(ctx)
}
