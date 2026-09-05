// This file coordinates the package's business rules and persistence operations behind a reusable API in the Product Master package.
package product

import (
	"context"
	"encoding/json"
	"errors"
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
	ErrNotFound     = errors.New("resource not found")
	ErrInvalidInput = errors.New("invalid input")
	ErrConflict     = errors.New("resource conflicts with existing data")
)

type Service struct {
	db         *pgxpool.Pool
	authorizer *authorization.Service
	audit      audit.Recorder
}

type Product struct {
	ID           string    `json:"id"`
	InternalCode string    `json:"internal_code"`
	Name         string    `json:"name"`
	Brand        *string   `json:"brand"`
	Variant      *string   `json:"variant"`
	Size         *string   `json:"size"`
	PackType     *string   `json:"pack_type"`
	UnitCount    *int      `json:"unit_count"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ProductInput struct {
	InternalCode string  `json:"internal_code"`
	Name         string  `json:"name"`
	Brand        *string `json:"brand"`
	Variant      *string `json:"variant"`
	Size         *string `json:"size"`
	PackType     *string `json:"pack_type"`
	UnitCount    *int    `json:"unit_count"`
	Status       string  `json:"status"`
}

type Marketplace struct {
	Key         string `json:"key"`
	DisplayName string `json:"display_name"`
}

type Mapping struct {
	ID                     string          `json:"id"`
	MarketplaceKey         string          `json:"marketplace_key"`
	ProductID              string          `json:"product_id"`
	SKU                    string          `json:"sku"`
	QuantityMultiplier     int             `json:"quantity_multiplier"`
	InterpretationMetadata json.RawMessage `json:"interpretation_metadata"`
	Status                 string          `json:"status"`
	CreatedAt              time.Time       `json:"created_at"`
	UpdatedAt              time.Time       `json:"updated_at"`
}

type MappingInput struct {
	MarketplaceKey         string         `json:"marketplace_key"`
	ProductID              string         `json:"product_id"`
	SKU                    string         `json:"sku"`
	QuantityMultiplier     int            `json:"quantity_multiplier"`
	InterpretationMetadata map[string]any `json:"interpretation_metadata"`
	Status                 string         `json:"status"`
}

type Resolution struct {
	Status  string   `json:"status"`
	Mapping *Mapping `json:"mapping,omitempty"`
	Product *Product `json:"product,omitempty"`
}

func NewService(db *pgxpool.Pool, authorizer *authorization.Service) *Service {
	return &Service{db: db, authorizer: authorizer}
}

func (s *Service) ListMarketplaces(ctx context.Context, principal auth.Principal) ([]Marketplace, error) {
	if err := s.authorizer.RequirePermission(ctx, principal, "products.view"); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT key, display_name FROM marketplaces WHERE status = 'active' ORDER BY display_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Marketplace, 0)
	for rows.Next() {
		var item Marketplace
		if err := rows.Scan(&item.Key, &item.DisplayName); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) ListProducts(ctx context.Context, principal auth.Principal, query, status string) ([]Product, error) {
	if err := s.authorizer.RequirePermission(ctx, principal, "products.view"); err != nil {
		return nil, err
	}
	query, status = strings.TrimSpace(query), strings.TrimSpace(status)
	if status != "" && status != "active" && status != "inactive" {
		return nil, ErrInvalidInput
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, internal_code, name, brand, variant, size, pack_type, unit_count, status, created_at, updated_at
		FROM products
		WHERE company_id = $1 AND ($2 = '' OR status = $2)
		  AND ($3 = '' OR internal_code ILIKE '%' || $3 || '%' OR name ILIKE '%' || $3 || '%')
		ORDER BY name, internal_code, id LIMIT 200`, principal.CompanyID, status, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Product, 0)
	for rows.Next() {
		var item Product
		if err := scanProduct(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) GetProduct(ctx context.Context, principal auth.Principal, id string) (Product, error) {
	if err := s.authorizer.RequirePermission(ctx, principal, "products.view"); err != nil {
		return Product{}, err
	}
	var item Product
	err := scanProduct(s.db.QueryRow(ctx, `SELECT id, internal_code, name, brand, variant, size, pack_type, unit_count, status, created_at, updated_at FROM products WHERE company_id=$1 AND id=$2`, principal.CompanyID, id), &item)
	return item, mapDBError(err)
}

func (s *Service) CreateProduct(ctx context.Context, principal auth.Principal, input ProductInput) (Product, error) {
	if err := s.authorizer.RequirePermission(ctx, principal, "products.manage"); err != nil {
		return Product{}, err
	}
	if !normalizeProductInput(&input, true) {
		return Product{}, ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Product{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var item Product
	err = scanProduct(tx.QueryRow(ctx, `INSERT INTO products (company_id,internal_code,name,brand,variant,size,pack_type,unit_count,status) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id,internal_code,name,brand,variant,size,pack_type,unit_count,status,created_at,updated_at`, principal.CompanyID, input.InternalCode, input.Name, input.Brand, input.Variant, input.Size, input.PackType, input.UnitCount, input.Status), &item)
	if err != nil {
		return Product{}, mapDBError(err)
	}
	if err := s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, "product.created", "product", item.ID, map[string]any{"internal_code": item.InternalCode, "name": item.Name}); err != nil {
		return Product{}, err
	}
	return item, tx.Commit(ctx)
}

func (s *Service) UpdateProduct(ctx context.Context, principal auth.Principal, id string, input ProductInput) (Product, error) {
	if err := s.authorizer.RequirePermission(ctx, principal, "products.manage"); err != nil {
		return Product{}, err
	}
	if !normalizeProductInput(&input, false) {
		return Product{}, ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Product{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var priorStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM products WHERE company_id=$1 AND id=$2 FOR UPDATE`, principal.CompanyID, id).Scan(&priorStatus); err != nil {
		return Product{}, mapDBError(err)
	}
	var item Product
	err = scanProduct(tx.QueryRow(ctx, `UPDATE products SET internal_code=$1,name=$2,brand=$3,variant=$4,size=$5,pack_type=$6,unit_count=$7,status=$8,updated_at=now() WHERE company_id=$9 AND id=$10 RETURNING id,internal_code,name,brand,variant,size,pack_type,unit_count,status,created_at,updated_at`, input.InternalCode, input.Name, input.Brand, input.Variant, input.Size, input.PackType, input.UnitCount, input.Status, principal.CompanyID, id), &item)
	if err != nil {
		return Product{}, mapDBError(err)
	}
	action := "product.updated"
	if priorStatus != input.Status {
		action = "product.status_changed"
	}
	if err := s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, action, "product", item.ID, map[string]any{"internal_code": item.InternalCode, "name": item.Name, "status": item.Status, "previous_status": priorStatus}); err != nil {
		return Product{}, err
	}
	return item, tx.Commit(ctx)
}

func (s *Service) ListMappings(ctx context.Context, principal auth.Principal, productID, marketplace, status string) ([]Mapping, error) {
	if err := s.authorizer.RequirePermission(ctx, principal, "products.view"); err != nil {
		return nil, err
	}
	productID, marketplace, status = strings.TrimSpace(productID), strings.TrimSpace(marketplace), strings.TrimSpace(status)
	if status != "" && status != "active" && status != "inactive" {
		return nil, ErrInvalidInput
	}
	rows, err := s.db.Query(ctx, `SELECT id,marketplace_key,product_id,sku,quantity_multiplier,interpretation_metadata,status,created_at,updated_at FROM sku_mappings WHERE company_id=$1 AND ($2='' OR product_id::text=$2) AND ($3='' OR marketplace_key=$3) AND ($4='' OR status=$4) ORDER BY marketplace_key,sku,id LIMIT 500`, principal.CompanyID, productID, marketplace, status)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()
	items := make([]Mapping, 0)
	for rows.Next() {
		var item Mapping
		if err := scanMapping(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) CreateMapping(ctx context.Context, principal auth.Principal, input MappingInput) (Mapping, error) {
	if err := s.authorizer.RequirePermission(ctx, principal, "products.manage"); err != nil {
		return Mapping{}, err
	}
	if !normalizeMappingInput(&input, true) {
		return Mapping{}, ErrInvalidInput
	}
	metadata, err := json.Marshal(input.InterpretationMetadata)
	if err != nil {
		return Mapping{}, ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Mapping{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var item Mapping
	err = scanMapping(tx.QueryRow(ctx, `INSERT INTO sku_mappings (company_id,marketplace_key,product_id,sku,quantity_multiplier,interpretation_metadata,status) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id,marketplace_key,product_id,sku,quantity_multiplier,interpretation_metadata,status,created_at,updated_at`, principal.CompanyID, input.MarketplaceKey, input.ProductID, input.SKU, input.QuantityMultiplier, metadata, input.Status), &item)
	if err != nil {
		return Mapping{}, mapDBError(err)
	}
	if err := s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, "sku_mapping.created", "sku_mapping", item.ID, map[string]any{"marketplace": item.MarketplaceKey, "sku": item.SKU, "product_id": item.ProductID}); err != nil {
		return Mapping{}, err
	}
	return item, tx.Commit(ctx)
}

func (s *Service) UpdateMapping(ctx context.Context, principal auth.Principal, id string, input MappingInput) (Mapping, error) {
	if err := s.authorizer.RequirePermission(ctx, principal, "products.manage"); err != nil {
		return Mapping{}, err
	}
	if !normalizeMappingInput(&input, false) {
		return Mapping{}, ErrInvalidInput
	}
	metadata, err := json.Marshal(input.InterpretationMetadata)
	if err != nil {
		return Mapping{}, ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Mapping{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var priorStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM sku_mappings WHERE company_id=$1 AND id=$2 FOR UPDATE`, principal.CompanyID, id).Scan(&priorStatus); err != nil {
		return Mapping{}, mapDBError(err)
	}
	var item Mapping
	err = scanMapping(tx.QueryRow(ctx, `UPDATE sku_mappings SET marketplace_key=$1,product_id=$2,sku=$3,quantity_multiplier=$4,interpretation_metadata=$5,status=$6,updated_at=now() WHERE company_id=$7 AND id=$8 RETURNING id,marketplace_key,product_id,sku,quantity_multiplier,interpretation_metadata,status,created_at,updated_at`, input.MarketplaceKey, input.ProductID, input.SKU, input.QuantityMultiplier, metadata, input.Status, principal.CompanyID, id), &item)
	if err != nil {
		return Mapping{}, mapDBError(err)
	}
	action := "sku_mapping.updated"
	if priorStatus != input.Status {
		action = "sku_mapping.status_changed"
	}
	if err := s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, action, "sku_mapping", item.ID, map[string]any{"marketplace": item.MarketplaceKey, "sku": item.SKU, "product_id": item.ProductID, "status": item.Status, "previous_status": priorStatus}); err != nil {
		return Mapping{}, err
	}
	return item, tx.Commit(ctx)
}

func (s *Service) Resolve(ctx context.Context, principal auth.Principal, marketplace, sku string) (Resolution, error) {
	if err := s.authorizer.RequirePermission(ctx, principal, "products.view"); err != nil {
		return Resolution{}, err
	}
	marketplace, sku = strings.TrimSpace(marketplace), strings.TrimSpace(sku)
	if marketplace == "" || sku == "" {
		return Resolution{}, ErrInvalidInput
	}
	var mapping Mapping
	var product Product
	err := s.db.QueryRow(ctx, `SELECT m.id,m.marketplace_key,m.product_id,m.sku,m.quantity_multiplier,m.interpretation_metadata,m.status,m.created_at,m.updated_at,p.id,p.internal_code,p.name,p.brand,p.variant,p.size,p.pack_type,p.unit_count,p.status,p.created_at,p.updated_at FROM sku_mappings m JOIN products p ON p.company_id=m.company_id AND p.id=m.product_id WHERE m.company_id=$1 AND m.marketplace_key=$2 AND m.sku=$3 AND m.status='active' AND p.status='active'`, principal.CompanyID, marketplace, sku).Scan(&mapping.ID, &mapping.MarketplaceKey, &mapping.ProductID, &mapping.SKU, &mapping.QuantityMultiplier, &mapping.InterpretationMetadata, &mapping.Status, &mapping.CreatedAt, &mapping.UpdatedAt, &product.ID, &product.InternalCode, &product.Name, &product.Brand, &product.Variant, &product.Size, &product.PackType, &product.UnitCount, &product.Status, &product.CreatedAt, &product.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Resolution{Status: "unresolved"}, nil
	}
	if err != nil {
		return Resolution{}, err
	}
	return Resolution{Status: "resolved", Mapping: &mapping, Product: &product}, nil
}

type scanner interface{ Scan(...any) error }

func scanProduct(row scanner, p *Product) error {
	return row.Scan(&p.ID, &p.InternalCode, &p.Name, &p.Brand, &p.Variant, &p.Size, &p.PackType, &p.UnitCount, &p.Status, &p.CreatedAt, &p.UpdatedAt)
}
func scanMapping(row scanner, m *Mapping) error {
	return row.Scan(&m.ID, &m.MarketplaceKey, &m.ProductID, &m.SKU, &m.QuantityMultiplier, &m.InterpretationMetadata, &m.Status, &m.CreatedAt, &m.UpdatedAt)
}
func normalizeProductInput(i *ProductInput, create bool) bool {
	i.InternalCode, i.Name = strings.TrimSpace(i.InternalCode), strings.TrimSpace(i.Name)
	i.Brand = trimOptional(i.Brand)
	i.Variant = trimOptional(i.Variant)
	i.Size = trimOptional(i.Size)
	i.PackType = trimOptional(i.PackType)
	if create && i.Status == "" {
		i.Status = "active"
	}
	return i.InternalCode != "" && i.Name != "" && (i.Status == "active" || i.Status == "inactive") && (i.UnitCount == nil || *i.UnitCount > 0)
}
func normalizeMappingInput(i *MappingInput, create bool) bool {
	i.MarketplaceKey, i.ProductID, i.SKU = strings.TrimSpace(i.MarketplaceKey), strings.TrimSpace(i.ProductID), strings.TrimSpace(i.SKU)
	if i.QuantityMultiplier == 0 {
		i.QuantityMultiplier = 1
	}
	if create && i.Status == "" {
		i.Status = "active"
	}
	if i.InterpretationMetadata == nil {
		i.InterpretationMetadata = map[string]any{}
	}
	return i.MarketplaceKey != "" && i.ProductID != "" && i.SKU != "" && i.QuantityMultiplier > 0 && (i.Status == "active" || i.Status == "inactive")
}
func trimOptional(v *string) *string {
	if v == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*v)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
func mapDBError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" {
			return ErrConflict
		}
		if pgErr.Code == "23503" || pgErr.Code == "22P02" || pgErr.Code == "23514" {
			return ErrInvalidInput
		}
	}
	return err
}
