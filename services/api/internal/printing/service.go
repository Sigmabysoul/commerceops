// Package printing owns reusable PDF assets, registered printers, and the
// canonical queue for physical print delivery. It intentionally has no
// dependency on Inventory: every operation in this package is stock-neutral.
package printing

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/audit"
	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/platform/objectstorage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const MaxPDFBytes = 20 << 20
const MaxCopies = 100
const LargeCopyThreshold = 20

var (
	ErrNotFound          = errors.New("printing resource not found")
	ErrInvalidInput      = errors.New("invalid printing input")
	ErrConflict          = errors.New("printing conflict")
	ErrInvalidCredential = errors.New("invalid agent credential")
	ErrInvalidState      = errors.New("invalid print job state")
	uuidRE               = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89aAbB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
	pagesRE              = regexp.MustCompile(`(?m)^Pages:\s+([0-9]+)\s*$`)
)

type Service struct {
	db      *pgxpool.Pool
	authz   *authorization.Service
	storage objectstorage.Storage
	audit   audit.Recorder
}

type AgentPrincipal struct {
	CompanyID string
	AgentID   string
}
type Agent struct {
	ID           string     `json:"id"`
	FriendlyName string     `json:"friendly_name"`
	Platform     string     `json:"platform"`
	Status       string     `json:"status"`
	LastSeenAt   *time.Time `json:"last_seen_at"`
	CreatedAt    time.Time  `json:"created_at"`
}
type AgentCredential struct {
	Agent      Agent  `json:"agent"`
	Credential string `json:"credential"`
}
type Printer struct {
	ID           string         `json:"id"`
	AgentID      string         `json:"agent_id"`
	FriendlyName string         `json:"friendly_name"`
	OSPrinterID  string         `json:"os_printer_id,omitempty"`
	Capabilities map[string]any `json:"capabilities"`
	Location     *string        `json:"location"`
	Status       string         `json:"status"`
	Enabled      bool           `json:"enabled"`
	LastSeenAt   *time.Time     `json:"last_seen_at"`
}
type Asset struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Category         string    `json:"category"`
	Description      *string   `json:"description"`
	SizeBytes        int64     `json:"size_bytes"`
	SHA256           string    `json:"sha256"`
	PageCount        int       `json:"page_count"`
	DefaultPrinterID *string   `json:"default_printer_id"`
	DefaultCopies    int       `json:"default_copies"`
	ProductID        *string   `json:"product_id"`
	Favorite         bool      `json:"favorite"`
	Active           bool      `json:"active"`
	CreatedAt        time.Time `json:"created_at"`
}
type Job struct {
	ID              string    `json:"id"`
	PrinterID       string    `json:"printer_id"`
	ArtifactID      *string   `json:"print_artifact_id"`
	AssetID         *string   `json:"print_library_asset_id"`
	Copies          int       `json:"copies"`
	OriginType      string    `json:"origin_type"`
	OriginReference string    `json:"origin_reference"`
	Status          string    `json:"status"`
	AttemptCount    int       `json:"attempt_count"`
	FailureCode     *string   `json:"failure_code"`
	FailureMessage  *string   `json:"failure_message"`
	SourceJobID     *string   `json:"source_printer_job_id"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
type LocalPrinter struct {
	OSPrinterID   string         `json:"os_printer_id"`
	SuggestedName string         `json:"suggested_name"`
	Capabilities  map[string]any `json:"capabilities"`
}
type CreateJobInput struct {
	PrinterID              string `json:"printer_id"`
	AssetID                string `json:"asset_id"`
	Copies                 int    `json:"copies"`
	LargeQuantityConfirmed bool   `json:"large_quantity_confirmed"`
	IdempotencyKey         string `json:"idempotency_key"`
}
type QueueArtifactInput struct {
	PrinterID      string `json:"printer_id"`
	ArtifactID     string `json:"artifact_id"`
	Copies         int    `json:"copies"`
	IdempotencyKey string `json:"idempotency_key"`
}
type UpdatePrinterInput struct {
	FriendlyName string  `json:"friendly_name"`
	Location     *string `json:"location"`
	Enabled      bool    `json:"enabled"`
}
type UpdateAssetInput struct {
	Name             string  `json:"name"`
	Category         string  `json:"category"`
	Description      *string `json:"description"`
	DefaultPrinterID *string `json:"default_printer_id"`
	DefaultCopies    int     `json:"default_copies"`
	ProductID        *string `json:"product_id"`
	Favorite         bool    `json:"favorite"`
	Active           bool    `json:"active"`
}
type Claim struct {
	Job            Job    `json:"job"`
	LeaseToken     string `json:"lease_token"`
	ArtifactSHA256 string `json:"artifact_sha256"`
	ArtifactSize   int64  `json:"artifact_size"`
	OSPrinterID    string `json:"os_printer_id"`
}

func NewService(db *pgxpool.Pool, authz *authorization.Service, storage objectstorage.Storage) *Service {
	return &Service{db: db, authz: authz, storage: storage}
}

func (s *Service) CreateAgent(ctx context.Context, p auth.Principal, name, platform string) (AgentCredential, error) {
	if err := s.authz.RequirePermission(ctx, p, "printers.manage"); err != nil {
		return AgentCredential{}, err
	}
	name = strings.TrimSpace(name)
	platform = strings.TrimSpace(platform)
	if name == "" || len(name) > 120 || platform != "linux_cups" {
		return AgentCredential{}, ErrInvalidInput
	}
	secret, hash, err := newSecret()
	if err != nil {
		return AgentCredential{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return AgentCredential{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var a Agent
	err = tx.QueryRow(ctx, `INSERT INTO printer_agents(company_id,friendly_name,platform) VALUES($1,$2,$3) RETURNING id,friendly_name,platform,status,last_seen_at,created_at`, p.CompanyID, name, platform).Scan(&a.ID, &a.FriendlyName, &a.Platform, &a.Status, &a.LastSeenAt, &a.CreatedAt)
	if err != nil {
		return AgentCredential{}, mapConflict(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO printer_agent_credentials(company_id,agent_id,token_hash) VALUES($1,$2,$3)`, p.CompanyID, a.ID, hash[:]); err != nil {
		return AgentCredential{}, err
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "printer_agent.registered", "printer_agent", a.ID, map[string]any{"friendly_name": name, "platform": platform}); err != nil {
		return AgentCredential{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AgentCredential{}, err
	}
	return AgentCredential{Agent: a, Credential: secret}, nil
}

func (s *Service) ListAgents(ctx context.Context, p auth.Principal) ([]Agent, error) {
	if err := s.authz.RequirePermission(ctx, p, "printers.view"); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT id,friendly_name,platform,CASE WHEN revoked_at IS NOT NULL THEN 'revoked' WHEN last_seen_at>now()-interval '90 seconds' THEN 'online' ELSE 'offline' END,last_seen_at,created_at FROM printer_agents WHERE company_id=$1 ORDER BY friendly_name,id`, p.CompanyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Agent{}
	for rows.Next() {
		var item Agent
		if err = rows.Scan(&item.ID, &item.FriendlyName, &item.Platform, &item.Status, &item.LastSeenAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// RevokeAgent invalidates every credential and prevents all of the device's
// printers from receiving work in the same transaction.
func (s *Service) RevokeAgent(ctx context.Context, p auth.Principal, id string) error {
	if err := s.authz.RequirePermission(ctx, p, "printers.manage"); err != nil {
		return err
	}
	if !uuidRE.MatchString(id) {
		return ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	tag, err := tx.Exec(ctx, `UPDATE printer_agents SET status='revoked',revoked_at=now(),updated_at=now() WHERE company_id=$1 AND id=$2 AND revoked_at IS NULL`, p.CompanyID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	if _, err = tx.Exec(ctx, `UPDATE printer_agent_credentials SET revoked_at=now() WHERE company_id=$1 AND agent_id=$2 AND revoked_at IS NULL`, p.CompanyID, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE registered_printers SET status='offline',updated_at=now() WHERE company_id=$1 AND agent_id=$2`, p.CompanyID, id); err != nil {
		return err
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "printer_agent.revoked", "printer_agent", id, map[string]any{}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) AuthenticateAgent(ctx context.Context, token string) (AgentPrincipal, error) {
	token = strings.TrimSpace(token)
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != 32 {
		return AgentPrincipal{}, ErrInvalidCredential
	}
	sum := sha256.Sum256([]byte(token))
	var p AgentPrincipal
	var stored []byte
	err = s.db.QueryRow(ctx, `SELECT c.company_id,c.agent_id,c.token_hash FROM printer_agent_credentials c JOIN printer_agents a ON a.company_id=c.company_id AND a.id=c.agent_id WHERE c.token_hash=$1 AND c.revoked_at IS NULL AND (c.expires_at IS NULL OR c.expires_at>now()) AND a.revoked_at IS NULL`, sum[:]).Scan(&p.CompanyID, &p.AgentID, &stored)
	if err != nil || len(stored) != 32 || subtle.ConstantTimeCompare(sum[:], stored) != 1 {
		return AgentPrincipal{}, ErrInvalidCredential
	}
	_, _ = s.db.Exec(ctx, `UPDATE printer_agent_credentials SET last_used_at=now() WHERE token_hash=$1`, sum[:])
	return p, nil
}

func (s *Service) ListPrinters(ctx context.Context, p auth.Principal) ([]Printer, error) {
	if err := s.authz.RequirePermission(ctx, p, "printers.view"); err != nil {
		return nil, err
	}
	items, err := s.listPrinters(ctx, p.CompanyID)
	// OS identifiers belong to the agent boundary. Browser users select only a
	// friendly name and opaque CommerceOps printer UUID.
	for index := range items {
		items[index].OSPrinterID = ""
	}
	return items, err
}

// UpdatePrinter changes only CommerceOps-owned presentation and availability
// settings. The OS identifier remains agent-reported and cannot be changed by
// a browser request.
func (s *Service) UpdatePrinter(ctx context.Context, p auth.Principal, id string, input UpdatePrinterInput) (Printer, error) {
	if err := s.authz.RequirePermission(ctx, p, "printers.manage"); err != nil {
		return Printer{}, err
	}
	input.FriendlyName = strings.TrimSpace(input.FriendlyName)
	if !uuidRE.MatchString(id) || input.FriendlyName == "" || len(input.FriendlyName) > 120 {
		return Printer{}, ErrInvalidInput
	}
	if input.Location != nil {
		trimmed := strings.TrimSpace(*input.Location)
		if len(trimmed) > 255 {
			return Printer{}, ErrInvalidInput
		}
		if trimmed == "" {
			input.Location = nil
		} else {
			input.Location = &trimmed
		}
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Printer{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var item Printer
	var raw []byte
	err = tx.QueryRow(ctx, `UPDATE registered_printers SET friendly_name=$3,location=$4,enabled=$5,updated_at=now() WHERE company_id=$1 AND id=$2 RETURNING id,agent_id,friendly_name,os_printer_id,capabilities,location,status,enabled,last_seen_at`, p.CompanyID, id, input.FriendlyName, input.Location, input.Enabled).Scan(&item.ID, &item.AgentID, &item.FriendlyName, &item.OSPrinterID, &raw, &item.Location, &item.Status, &item.Enabled, &item.LastSeenAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Printer{}, ErrNotFound
	}
	if err != nil {
		return Printer{}, mapConflict(err)
	}
	if err = json.Unmarshal(raw, &item.Capabilities); err != nil {
		return Printer{}, fmt.Errorf("decode printer capabilities: %w", err)
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "printer.updated", "registered_printer", id, map[string]any{"friendly_name": item.FriendlyName, "enabled": item.Enabled, "location": item.Location}); err != nil {
		return Printer{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Printer{}, err
	}
	return item, nil
}
func (s *Service) listPrinters(ctx context.Context, company string) ([]Printer, error) {
	query := `SELECT id,agent_id,friendly_name,os_printer_id,capabilities,location,CASE WHEN last_seen_at>now()-interval '90 seconds' THEN status ELSE 'offline' END,enabled,last_seen_at FROM registered_printers WHERE company_id=$1`
	args := []any{company}
	query += ` ORDER BY friendly_name,id`
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Printer{}
	for rows.Next() {
		var x Printer
		var raw []byte
		if err = rows.Scan(&x.ID, &x.AgentID, &x.FriendlyName, &x.OSPrinterID, &raw, &x.Location, &x.Status, &x.Enabled, &x.LastSeenAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &x.Capabilities); err != nil {
			return nil, fmt.Errorf("decode printer capabilities: %w", err)
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Service) Heartbeat(ctx context.Context, p AgentPrincipal, printers []LocalPrinter) ([]Printer, error) {
	if len(printers) > 100 {
		return nil, ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if tag, err := tx.Exec(ctx, `UPDATE printer_agents SET status='online',last_seen_at=now(),updated_at=now() WHERE company_id=$1 AND id=$2 AND revoked_at IS NULL`, p.CompanyID, p.AgentID); err != nil || tag.RowsAffected() != 1 {
		if err != nil {
			return nil, err
		}
		return nil, ErrInvalidCredential
	}
	seen := map[string]bool{}
	for _, item := range printers {
		item.OSPrinterID = strings.TrimSpace(item.OSPrinterID)
		item.SuggestedName = strings.TrimSpace(item.SuggestedName)
		if item.OSPrinterID == "" || len(item.OSPrinterID) > 255 || item.SuggestedName == "" || len(item.SuggestedName) > 120 || seen[item.OSPrinterID] {
			return nil, ErrInvalidInput
		}
		seen[item.OSPrinterID] = true
		raw, marshalErr := json.Marshal(item.Capabilities)
		if marshalErr != nil {
			return nil, fmt.Errorf("encode printer capabilities: %w", marshalErr)
		}
		_, err = tx.Exec(ctx, `INSERT INTO registered_printers(company_id,agent_id,friendly_name,os_printer_id,capabilities,status,last_seen_at) VALUES($1,$2,$3,$4,$5,'online',now()) ON CONFLICT(company_id,agent_id,os_printer_id) DO UPDATE SET capabilities=EXCLUDED.capabilities,status='online',last_seen_at=now(),updated_at=now()`, p.CompanyID, p.AgentID, item.SuggestedName, item.OSPrinterID, raw)
		if err != nil {
			return nil, mapConflict(err)
		}
	}
	_, err = tx.Exec(ctx, `UPDATE registered_printers SET status='offline',updated_at=now() WHERE company_id=$1 AND agent_id=$2 AND NOT(os_printer_id=ANY($3::text[]))`, p.CompanyID, p.AgentID, mapKeys(seen))
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT id,agent_id,friendly_name,os_printer_id,capabilities,location,status,enabled,last_seen_at FROM registered_printers WHERE company_id=$1 AND agent_id=$2 ORDER BY friendly_name,id`, p.CompanyID, p.AgentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Printer{}
	for rows.Next() {
		var x Printer
		var raw []byte
		if err = rows.Scan(&x.ID, &x.AgentID, &x.FriendlyName, &x.OSPrinterID, &raw, &x.Location, &x.Status, &x.Enabled, &x.LastSeenAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &x.Capabilities); err != nil {
			return nil, fmt.Errorf("decode printer capabilities: %w", err)
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Service) CreateAsset(ctx context.Context, p auth.Principal, name, category, description string, defaultPrinterID *string, defaultCopies int, productID *string, favorite bool, filename string, data []byte) (Asset, error) {
	if err := s.authz.RequirePermission(ctx, p, "print_library.manage"); err != nil {
		return Asset{}, err
	}
	name = strings.TrimSpace(name)
	category = strings.TrimSpace(category)
	description = strings.TrimSpace(description)
	if name == "" || len(name) > 120 || category == "" || len(category) > 80 || len(description) > 1000 || defaultCopies < 1 || defaultCopies > MaxCopies || len(data) == 0 || len(data) > MaxPDFBytes || !bytes.HasPrefix(data, []byte("%PDF-")) {
		return Asset{}, ErrInvalidInput
	}
	pages, err := validatePDF(ctx, data)
	if err != nil {
		return Asset{}, ErrInvalidInput
	}
	sum := sha256.Sum256(data)
	id, err := randomUUID()
	if err != nil {
		return Asset{}, err
	}
	key := path.Join(p.CompanyID, "print-library", id+".pdf")
	if err = s.storage.Put(ctx, key, bytes.NewReader(data), int64(len(data)), "application/pdf"); err != nil {
		return Asset{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = s.storage.Delete(ctx, key)
		}
	}()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Asset{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var a Asset
	var desc any
	if description != "" {
		desc = description
	}
	err = tx.QueryRow(ctx, `INSERT INTO print_library_assets(id,company_id,name,category,description,storage_key,size_bytes,sha256,page_count,default_printer_id,default_copies,product_id,favorite,uploaded_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING id,name,category,description,size_bytes,sha256,page_count,default_printer_id,default_copies,product_id,favorite,active,created_at`, id, p.CompanyID, name, category, desc, key, len(data), hex.EncodeToString(sum[:]), pages, defaultPrinterID, defaultCopies, productID, favorite, p.UserID).Scan(&a.ID, &a.Name, &a.Category, &a.Description, &a.SizeBytes, &a.SHA256, &a.PageCount, &a.DefaultPrinterID, &a.DefaultCopies, &a.ProductID, &a.Favorite, &a.Active, &a.CreatedAt)
	if err != nil {
		return Asset{}, mapConflict(err)
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "print_library.uploaded", "print_library_asset", a.ID, map[string]any{"name": name, "category": category, "sha256": a.SHA256, "page_count": pages}); err != nil {
		return Asset{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Asset{}, err
	}
	committed = true
	return a, nil
}

func (s *Service) ListAssets(ctx context.Context, p auth.Principal, search, category string) ([]Asset, error) {
	if err := s.authz.RequirePermission(ctx, p, "print_library.view"); err != nil {
		return nil, err
	}
	search = strings.TrimSpace(search)
	category = strings.TrimSpace(category)
	rows, err := s.db.Query(ctx, `SELECT id,name,category,description,size_bytes,sha256,page_count,default_printer_id,default_copies,product_id,favorite,active,created_at FROM print_library_assets WHERE company_id=$1 AND active AND ($2='' OR name ILIKE '%'||$2||'%' OR description ILIKE '%'||$2||'%') AND ($3='' OR category=$3) ORDER BY favorite DESC,name,id`, p.CompanyID, search, category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Asset{}
	for rows.Next() {
		var a Asset
		if err = rows.Scan(&a.ID, &a.Name, &a.Category, &a.Description, &a.SizeBytes, &a.SHA256, &a.PageCount, &a.DefaultPrinterID, &a.DefaultCopies, &a.ProductID, &a.Favorite, &a.Active, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Service) ArchiveAsset(ctx context.Context, p auth.Principal, id string) error {
	if err := s.authz.RequirePermission(ctx, p, "print_library.manage"); err != nil {
		return err
	}
	if !uuidRE.MatchString(id) {
		return ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	tag, err := tx.Exec(ctx, `UPDATE print_library_assets SET active=false,updated_at=now() WHERE company_id=$1 AND id=$2 AND active`, p.CompanyID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "print_library.archived", "print_library_asset", id, map[string]any{}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// UpdateAsset changes reusable metadata only. The PDF and its hash remain
// immutable so queued and historical jobs always identify exact content.
func (s *Service) UpdateAsset(ctx context.Context, p auth.Principal, id string, input UpdateAssetInput) (Asset, error) {
	if err := s.authz.RequirePermission(ctx, p, "print_library.manage"); err != nil {
		return Asset{}, err
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Category = strings.TrimSpace(input.Category)
	if !uuidRE.MatchString(id) || input.Name == "" || len(input.Name) > 120 || input.Category == "" || len(input.Category) > 80 || input.DefaultCopies < 1 || input.DefaultCopies > MaxCopies {
		return Asset{}, ErrInvalidInput
	}
	if input.Description != nil {
		value := strings.TrimSpace(*input.Description)
		if len(value) > 1000 {
			return Asset{}, ErrInvalidInput
		}
		if value == "" {
			input.Description = nil
		} else {
			input.Description = &value
		}
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Asset{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var item Asset
	err = tx.QueryRow(ctx, `UPDATE print_library_assets SET name=$3,category=$4,description=$5,default_printer_id=$6,default_copies=$7,product_id=$8,favorite=$9,active=$10,updated_at=now() WHERE company_id=$1 AND id=$2 RETURNING id,name,category,description,size_bytes,sha256,page_count,default_printer_id,default_copies,product_id,favorite,active,created_at`, p.CompanyID, id, input.Name, input.Category, input.Description, input.DefaultPrinterID, input.DefaultCopies, input.ProductID, input.Favorite, input.Active).Scan(&item.ID, &item.Name, &item.Category, &item.Description, &item.SizeBytes, &item.SHA256, &item.PageCount, &item.DefaultPrinterID, &item.DefaultCopies, &item.ProductID, &item.Favorite, &item.Active, &item.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Asset{}, ErrNotFound
	}
	if err != nil {
		return Asset{}, mapConflict(err)
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "print_library.updated", "print_library_asset", id, map[string]any{"name": item.Name, "category": item.Category, "favorite": item.Favorite, "active": item.Active}); err != nil {
		return Asset{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Asset{}, err
	}
	return item, nil
}

func (s *Service) CreateQuickJob(ctx context.Context, p auth.Principal, in CreateJobInput) (Job, bool, error) {
	if err := s.authz.RequirePermission(ctx, p, "printing.print"); err != nil {
		return Job{}, false, err
	}
	if !uuidRE.MatchString(in.AssetID) {
		return Job{}, false, ErrInvalidInput
	}
	if in.Copies > LargeCopyThreshold && !in.LargeQuantityConfirmed {
		return Job{}, false, ErrInvalidInput
	}
	return s.createJob(ctx, p, in.PrinterID, nil, &in.AssetID, in.Copies, "quick_print", in.AssetID, in.IdempotencyKey, nil)
}
func (s *Service) QueueArtifact(ctx context.Context, p auth.Principal, in QueueArtifactInput) (Job, bool, error) {
	if err := s.authz.RequirePermission(ctx, p, "printing.print"); err != nil {
		return Job{}, false, err
	}
	if !uuidRE.MatchString(in.ArtifactID) {
		return Job{}, false, ErrInvalidInput
	}
	var origin, ref string
	err := s.db.QueryRow(ctx, `SELECT CASE WHEN pj.source_print_job_id IS NULL THEN 'ecommerce_batch' ELSE 'ecommerce_reprint' END,pj.id::text FROM print_artifacts pa JOIN print_jobs pj ON pj.company_id=pa.company_id AND pj.id=pa.print_job_id WHERE pa.company_id=$1 AND pa.id=$2`, p.CompanyID, in.ArtifactID).Scan(&origin, &ref)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, false, ErrNotFound
	}
	if err != nil {
		return Job{}, false, err
	}
	return s.createJob(ctx, p, in.PrinterID, &in.ArtifactID, nil, in.Copies, origin, ref, in.IdempotencyKey, nil)
}

func (s *Service) createJob(ctx context.Context, p auth.Principal, printer string, artifact, asset *string, copies int, origin, ref, key string, source *string) (Job, bool, error) {
	// The request hash makes an idempotency key semantic: an exact replay returns
	// the first job, while changing printer/source/copies is a visible conflict.
	key = strings.TrimSpace(key)
	if !uuidRE.MatchString(printer) || copies < 1 || copies > MaxCopies || key == "" || len(key) > 128 {
		return Job{}, false, ErrInvalidInput
	}
	payload, err := json.Marshal([]any{printer, artifact, asset, copies, origin, ref, source})
	if err != nil {
		return Job{}, false, fmt.Errorf("encode print job identity: %w", err)
	}
	sum := sha256.Sum256(payload)
	hash := hex.EncodeToString(sum[:])
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Job{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO printer_jobs(company_id,requested_by,printer_id,print_artifact_id,print_library_asset_id,copies,origin_type,origin_reference,idempotency_key,request_hash,source_printer_job_id) SELECT $1,$2,rp.id,$3,$4,$5,$6,$7,$8,$9,$10 FROM registered_printers rp WHERE rp.company_id=$1 AND rp.id=$11 AND rp.enabled AND rp.status='online' AND rp.last_seen_at>now()-interval '90 seconds' AND ($4::uuid IS NULL OR EXISTS(SELECT 1 FROM print_library_assets a WHERE a.company_id=$1 AND a.id=$4 AND a.active)) ON CONFLICT(company_id,idempotency_key) DO NOTHING RETURNING id`, p.CompanyID, p.UserID, artifact, asset, copies, origin, ref, key, hash, source, printer).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		var existingHash string
		if e := tx.QueryRow(ctx, `SELECT id,request_hash FROM printer_jobs WHERE company_id=$1 AND idempotency_key=$2`, p.CompanyID, key).Scan(&id, &existingHash); e == nil {
			if existingHash != hash {
				return Job{}, false, ErrConflict
			}
			if commitErr := tx.Commit(ctx); commitErr != nil {
				return Job{}, false, commitErr
			}
			j, e := s.getJob(ctx, p.CompanyID, id)
			return j, true, e
		}
		return Job{}, false, ErrNotFound
	}
	if err != nil {
		return Job{}, false, mapConflict(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO printer_job_events(company_id,printer_job_id,event_type,actor_user_id) VALUES($1,$2,'queued',$3)`, p.CompanyID, id, p.UserID); err != nil {
		return Job{}, false, err
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "printing.requested", "printer_job", id, map[string]any{"printer_id": printer, "copies": copies, "origin_type": origin, "origin_reference": ref}); err != nil {
		return Job{}, false, err
	}
	if source != nil {
		metadata := map[string]any{"source_printer_job_id": *source}
		if _, err = tx.Exec(ctx, `INSERT INTO printer_job_events(company_id,printer_job_id,event_type,actor_user_id,metadata) VALUES($1,$2,'retried',$3,jsonb_build_object('source_printer_job_id',$4::text))`, p.CompanyID, id, p.UserID, *source); err != nil {
			return Job{}, false, err
		}
		if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "printing.retried", "printer_job", id, metadata); err != nil {
			return Job{}, false, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Job{}, false, err
	}
	j, err := s.getJob(ctx, p.CompanyID, id)
	return j, false, err
}

func (s *Service) ListJobs(ctx context.Context, p auth.Principal) ([]Job, error) {
	if err := s.authz.RequirePermission(ctx, p, "printing.print"); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, jobSelect+` WHERE company_id=$1 ORDER BY created_at DESC LIMIT 200`, p.CompanyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		j, e := scanJob(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
func (s *Service) Cancel(ctx context.Context, p auth.Principal, id string) (Job, error) {
	if err := s.authz.RequirePermission(ctx, p, "printing.print"); err != nil {
		return Job{}, err
	}
	if !uuidRE.MatchString(id) {
		return Job{}, ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	tag, err := tx.Exec(ctx, `UPDATE printer_jobs SET status='cancelled',cancelled_at=now(),updated_at=now() WHERE company_id=$1 AND id=$2 AND status='queued'`, p.CompanyID, id)
	if err != nil {
		return Job{}, err
	}
	if tag.RowsAffected() != 1 {
		return Job{}, ErrInvalidState
	}
	_, err = tx.Exec(ctx, `INSERT INTO printer_job_events(company_id,printer_job_id,event_type,actor_user_id) VALUES($1,$2,'cancelled',$3)`, p.CompanyID, id, p.UserID)
	if err == nil {
		err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "printing.cancelled", "printer_job", id, map[string]any{})
	}
	if err != nil {
		return Job{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Job{}, err
	}
	return s.getJob(ctx, p.CompanyID, id)
}
func (s *Service) Retry(ctx context.Context, p auth.Principal, id, key string) (Job, bool, error) {
	if err := s.authz.RequirePermission(ctx, p, "printing.reprint"); err != nil {
		return Job{}, false, err
	}
	source, err := s.getJob(ctx, p.CompanyID, id)
	if err != nil {
		return Job{}, false, err
	}
	if source.Status != "failed" {
		return Job{}, false, ErrInvalidState
	}
	return s.createJob(ctx, p, source.PrinterID, source.ArtifactID, source.AssetID, source.Copies, source.OriginType, source.OriginReference, key, &source.ID)
}

func (s *Service) Claim(ctx context.Context, p AgentPrincipal) (Claim, error) {
	// SKIP LOCKED lets multiple devices poll concurrently without delivering one
	// job twice. The random lease is also required for download and state reports.
	token, hash, err := newSecret()
	if err != nil {
		return Claim{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Claim{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	// Expiry is ambiguous at the physical boundary. Mark it failed for operator
	// review instead of automatically queuing a potentially printed job again.
	expired, err := tx.Query(ctx, `UPDATE printer_jobs SET status='failed',failure_code='lease_expired',failure_message='Agent lease expired; inspect the printer before retrying',failed_at=now(),updated_at=now() WHERE company_id=$1 AND lease_agent_id=$2 AND status IN ('claimed','printing') AND lease_expires_at<=now() RETURNING id`, p.CompanyID, p.AgentID)
	if err != nil {
		return Claim{}, err
	}
	expiredIDs := []string{}
	for expired.Next() {
		var expiredID string
		if err = expired.Scan(&expiredID); err != nil {
			expired.Close()
			return Claim{}, err
		}
		expiredIDs = append(expiredIDs, expiredID)
	}
	expired.Close()
	if err = expired.Err(); err != nil {
		return Claim{}, err
	}
	for _, expiredID := range expiredIDs {
		if _, err = tx.Exec(ctx, `INSERT INTO printer_job_events(company_id,printer_job_id,event_type,actor_agent_id) VALUES($1,$2,'lease_expired',$3)`, p.CompanyID, expiredID, p.AgentID); err != nil {
			return Claim{}, err
		}
	}
	var c Claim
	err = tx.QueryRow(ctx, `WITH candidate AS (SELECT j.id FROM printer_jobs j JOIN registered_printers rp ON rp.company_id=j.company_id AND rp.id=j.printer_id WHERE j.company_id=$1 AND rp.agent_id=$2 AND rp.enabled AND rp.status='online' AND rp.last_seen_at>now()-interval '90 seconds' AND j.status='queued' ORDER BY j.created_at,j.id FOR UPDATE OF j SKIP LOCKED LIMIT 1) UPDATE printer_jobs j SET status='claimed',lease_agent_id=$2,lease_token_hash=$3,lease_expires_at=now()+interval '2 minutes',attempt_count=attempt_count+1,claimed_at=now(),updated_at=now() FROM candidate WHERE j.id=candidate.id RETURNING j.id,j.printer_id,j.print_artifact_id,j.print_library_asset_id,j.copies,j.origin_type,j.origin_reference,j.status,j.attempt_count,j.failure_code,j.failure_message,j.source_printer_job_id,j.created_at,j.updated_at`, p.CompanyID, p.AgentID, hash[:]).Scan(&c.Job.ID, &c.Job.PrinterID, &c.Job.ArtifactID, &c.Job.AssetID, &c.Job.Copies, &c.Job.OriginType, &c.Job.OriginReference, &c.Job.Status, &c.Job.AttemptCount, &c.Job.FailureCode, &c.Job.FailureMessage, &c.Job.SourceJobID, &c.Job.CreatedAt, &c.Job.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return Claim{}, commitErr
		}
		return Claim{}, ErrNotFound
	}
	if err != nil {
		return Claim{}, err
	}
	err = tx.QueryRow(ctx, `SELECT COALESCE(pa.sha256,a.sha256),COALESCE(pa.size_bytes,a.size_bytes),rp.os_printer_id FROM printer_jobs j JOIN registered_printers rp ON rp.company_id=j.company_id AND rp.id=j.printer_id LEFT JOIN print_artifacts pa ON pa.company_id=j.company_id AND pa.id=j.print_artifact_id LEFT JOIN print_library_assets a ON a.company_id=j.company_id AND a.id=j.print_library_asset_id WHERE j.company_id=$1 AND j.id=$2`, p.CompanyID, c.Job.ID).Scan(&c.ArtifactSHA256, &c.ArtifactSize, &c.OSPrinterID)
	if err != nil {
		return Claim{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO printer_job_events(company_id,printer_job_id,event_type,actor_agent_id) VALUES($1,$2,'claimed',$3)`, p.CompanyID, c.Job.ID, p.AgentID)
	if err != nil {
		return Claim{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Claim{}, err
	}
	c.LeaseToken = token
	return c, nil
}

func (s *Service) AgentDownload(ctx context.Context, p AgentPrincipal, id, lease string) ([]byte, error) {
	// Storage keys never cross the API boundary; the server resolves the source
	// only after validating tenant, owning agent, job state, and live lease.
	if !uuidRE.MatchString(id) {
		return nil, ErrInvalidInput
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(lease)))
	var key string
	var size int64
	err := s.db.QueryRow(ctx, `SELECT COALESCE(pa.storage_key,a.storage_key),COALESCE(pa.size_bytes,a.size_bytes) FROM printer_jobs j JOIN registered_printers rp ON rp.company_id=j.company_id AND rp.id=j.printer_id LEFT JOIN print_artifacts pa ON pa.company_id=j.company_id AND pa.id=j.print_artifact_id LEFT JOIN print_library_assets a ON a.company_id=j.company_id AND a.id=j.print_library_asset_id WHERE j.company_id=$1 AND j.id=$2 AND rp.agent_id=$3 AND j.lease_agent_id=$3 AND j.lease_token_hash=$4 AND j.lease_expires_at>now() AND j.status IN ('claimed','printing')`, p.CompanyID, id, p.AgentID, sum[:]).Scan(&key, &size)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidCredential
	}
	if err != nil {
		return nil, err
	}
	r, err := s.storage.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, size+1))
	if err != nil || int64(len(data)) != size {
		return nil, ErrConflict
	}
	return data, nil
}

func (s *Service) Report(ctx context.Context, p AgentPrincipal, id, lease, status, code, message string) (Job, error) {
	if status != "printing" && status != "completed" && status != "failed" {
		return Job{}, ErrInvalidInput
	}
	if len(code) > 80 || len(message) > 1000 {
		return Job{}, ErrInvalidInput
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(lease)))
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var current string
	err = tx.QueryRow(ctx, `SELECT status FROM printer_jobs WHERE company_id=$1 AND id=$2 AND lease_agent_id=$3 AND lease_token_hash=$4 AND lease_expires_at>now() FOR UPDATE`, p.CompanyID, id, p.AgentID, sum[:]).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrInvalidCredential
	}
	if err != nil {
		return Job{}, err
	}
	if current == status || (current == "completed" && status == "completed") || (current == "failed" && status == "failed") {
		if err = tx.Commit(ctx); err != nil {
			return Job{}, err
		}
		return s.getJob(ctx, p.CompanyID, id)
	}
	allowed := (current == "claimed" && status == "printing") || ((current == "claimed" || current == "printing") && (status == "completed" || status == "failed"))
	if !allowed {
		return Job{}, ErrInvalidState
	}
	if status == "failed" && strings.TrimSpace(message) == "" {
		return Job{}, ErrInvalidInput
	}
	_, err = tx.Exec(ctx, `UPDATE printer_jobs SET status=$5,printing_at=CASE WHEN $5='printing' THEN now() ELSE printing_at END,completed_at=CASE WHEN $5='completed' THEN now() ELSE completed_at END,failed_at=CASE WHEN $5='failed' THEN now() ELSE failed_at END,failure_code=CASE WHEN $5='failed' THEN NULLIF($6::text,'') ELSE failure_code END,failure_message=CASE WHEN $5='failed' THEN $7::text ELSE failure_message END,lease_expires_at=CASE WHEN $5='printing' THEN now()+interval '2 minutes' ELSE lease_expires_at END,updated_at=now() WHERE company_id=$1 AND id=$2 AND lease_agent_id=$3 AND lease_token_hash=$4`, p.CompanyID, id, p.AgentID, sum[:], status, strings.TrimSpace(code), strings.TrimSpace(message))
	if err != nil {
		return Job{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO printer_job_events(company_id,printer_job_id,event_type,actor_agent_id,metadata) VALUES($1,$2,$3,$4,jsonb_build_object('failure_code',NULLIF($5::text,''),'failure_message',NULLIF($6::text,'')))`, p.CompanyID, id, status, p.AgentID, strings.TrimSpace(code), strings.TrimSpace(message))
	if err != nil {
		return Job{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_logs(company_id,actor_user_id,action,target_type,target_id,metadata) VALUES($1,NULL,$2,'printer_job',$3,jsonb_build_object('agent_id',$4::text,'status',$5::text))`, p.CompanyID, "printing."+status, id, p.AgentID, status)
	if err != nil {
		return Job{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Job{}, err
	}
	return s.getJob(ctx, p.CompanyID, id)
}

const jobSelect = `SELECT id,printer_id,print_artifact_id,print_library_asset_id,copies,origin_type,origin_reference,status,attempt_count,failure_code,failure_message,source_printer_job_id,created_at,updated_at FROM printer_jobs`

type scanner interface{ Scan(...any) error }

func scanJob(r scanner) (Job, error) {
	var j Job
	err := r.Scan(&j.ID, &j.PrinterID, &j.ArtifactID, &j.AssetID, &j.Copies, &j.OriginType, &j.OriginReference, &j.Status, &j.AttemptCount, &j.FailureCode, &j.FailureMessage, &j.SourceJobID, &j.CreatedAt, &j.UpdatedAt)
	return j, err
}
func (s *Service) getJob(ctx context.Context, company, id string) (Job, error) {
	j, err := scanJob(s.db.QueryRow(ctx, jobSelect+` WHERE company_id=$1 AND id=$2`, company, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	return j, err
}
func newSecret() (string, [32]byte, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", [32]byte{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	return token, sha256.Sum256([]byte(token)), nil
}
func randomUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
func validatePDF(ctx context.Context, data []byte) (int, error) {
	f, err := os.CreateTemp("", "commerceops-library-*.pdf")
	if err != nil {
		return 0, err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0o600); err == nil {
		_, err = f.Write(data)
	}
	closeErr := f.Close()
	if err != nil {
		return 0, err
	}
	if closeErr != nil {
		return 0, closeErr
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, "pdfinfo", name).CombinedOutput()
	if err != nil {
		return 0, err
	}
	m := pagesRE.FindSubmatch(out)
	if len(m) != 2 {
		return 0, ErrInvalidInput
	}
	pages, err := strconv.Atoi(string(m[1]))
	if err != nil || pages < 1 || pages > 500 {
		return 0, ErrInvalidInput
	}
	return pages, nil
}
func mapConflict(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "foreign key") {
		return ErrConflict
	}
	return err
}
func mapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
