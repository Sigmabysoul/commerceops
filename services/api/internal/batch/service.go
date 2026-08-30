package batch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/audit"
	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/platform/objectstorage"
	"github.com/commerceops/commerceops/services/api/internal/platform/pdfgenerator"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxBatchMembers = 500

var (
	ErrNotFound     = errors.New("batch not found")
	ErrInvalidInput = errors.New("invalid batch input")
	ErrConflict     = errors.New("batch conflicts with existing data")
	ErrIneligible   = errors.New("one or more orders are not eligible")
	ErrInvalidState = errors.New("invalid batch state transition")
	ErrUnresolved   = errors.New("batch contains unresolved items")
	uuidRE          = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89aAbB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
)

type Service struct {
	db         *pgxpool.Pool
	authorizer *authorization.Service
	audit      audit.Recorder
	storage    objectstorage.Storage
	generator  pdfgenerator.Generator
}

type Batch struct {
	ID              string         `json:"id"`
	MarketplaceKey  string         `json:"marketplace_key"`
	Status          string         `json:"status"`
	CreatedBy       string         `json:"created_by"`
	OrderCount      int            `json:"order_count"`
	UnresolvedCount int            `json:"unresolved_count"`
	ReadyAt         *time.Time     `json:"ready_at"`
	CancelledAt     *time.Time     `json:"cancelled_at"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	Members         []Member       `json:"members,omitempty"`
	ProductTotals   []ProductTotal `json:"product_totals,omitempty"`
}

type Member struct {
	OrderID            string  `json:"order_id"`
	Position           int     `json:"position"`
	SourceFileID       string  `json:"source_file_id"`
	ProcessingJobID    string  `json:"processing_job_id"`
	SourcePage         int     `json:"source_page"`
	MarketplaceOrderID *string `json:"marketplace_order_id"`
	AWB                *string `json:"awb"`
	Status             string  `json:"status"`
}

type ProductTotal struct {
	ProductID      string `json:"product_id"`
	InternalCode   string `json:"internal_code"`
	ProductName    string `json:"product_name"`
	TotalQuantity  int    `json:"total_quantity"`
	OrderLineCount int    `json:"order_line_count"`
}

type EligibleOrder struct {
	OrderID            string  `json:"order_id"`
	SourceFileID       string  `json:"source_file_id"`
	ProcessingJobID    string  `json:"processing_job_id"`
	SourcePage         int     `json:"source_page"`
	MarketplaceOrderID *string `json:"marketplace_order_id"`
	AWB                *string `json:"awb"`
	Status             string  `json:"status"`
	UnresolvedCount    int     `json:"unresolved_count"`
}

type CreateInput struct {
	MarketplaceKey string   `json:"marketplace_key"`
	OrderIDs       []string `json:"order_ids"`
	IdempotencyKey string   `json:"idempotency_key"`
}

func NewService(db *pgxpool.Pool, authorizer *authorization.Service) *Service {
	return &Service{db: db, authorizer: authorizer}
}

func NewPrintingService(db *pgxpool.Pool, authorizer *authorization.Service, storage objectstorage.Storage, generator pdfgenerator.Generator) *Service {
	return &Service{db: db, authorizer: authorizer, storage: storage, generator: generator}
}

func (s *Service) List(ctx context.Context, principal auth.Principal) ([]Batch, error) {
	if err := s.authorize(ctx, principal); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT b.id,b.marketplace_key,b.status,b.created_by,count(DISTINCT bm.marketplace_order_id),
		       count(*) FILTER (WHERE mo.id IS NOT NULL AND (mo.status<>'resolved' OR moi.id IS NULL OR moi.product_id IS NULL OR moi.quantity IS NULL OR moi.resolution_status<>'resolved')),
		       b.ready_at,b.cancelled_at,b.created_at,b.updated_at
		FROM batches b
		LEFT JOIN batch_members bm ON bm.company_id=b.company_id AND bm.batch_id=b.id
		LEFT JOIN marketplace_orders mo ON mo.company_id=bm.company_id AND mo.id=bm.marketplace_order_id
		LEFT JOIN marketplace_order_items moi ON moi.company_id=mo.company_id AND moi.order_id=mo.id
		WHERE b.company_id=$1 GROUP BY b.id ORDER BY b.created_at DESC,b.id DESC LIMIT 200`, principal.CompanyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Batch, 0)
	for rows.Next() {
		var item Batch
		if err := scanBatch(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) EligibleOrders(ctx context.Context, principal auth.Principal, marketplace string) ([]EligibleOrder, error) {
	if err := s.authorize(ctx, principal); err != nil {
		return nil, err
	}
	marketplace = strings.TrimSpace(marketplace)
	if marketplace != "flipkart" {
		return nil, ErrInvalidInput
	}
	rows, err := s.db.Query(ctx, `
		SELECT mo.id,mo.source_file_id,mo.processing_job_id,mo.source_page,mo.marketplace_order_id,mo.awb,mo.status,
		       count(*) FILTER (WHERE moi.id IS NULL OR moi.product_id IS NULL OR moi.quantity IS NULL OR moi.resolution_status<>'resolved')
		FROM marketplace_orders mo
		JOIN processing_jobs pj ON pj.company_id=mo.company_id AND pj.id=mo.processing_job_id
		LEFT JOIN marketplace_order_items moi ON moi.company_id=mo.company_id AND moi.order_id=mo.id
		LEFT JOIN batch_members bm ON bm.company_id=mo.company_id AND bm.marketplace_order_id=mo.id
		WHERE mo.company_id=$1 AND mo.marketplace_key=$2 AND pj.status IN ('processed','needs_review')
		  AND mo.status<>'duplicate' AND bm.marketplace_order_id IS NULL
		GROUP BY mo.id ORDER BY mo.created_at,mo.source_page,mo.id LIMIT 500`, principal.CompanyID, marketplace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]EligibleOrder, 0)
	for rows.Next() {
		var item EligibleOrder
		if err := rows.Scan(&item.OrderID, &item.SourceFileID, &item.ProcessingJobID, &item.SourcePage, &item.MarketplaceOrderID, &item.AWB, &item.Status, &item.UnresolvedCount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) Create(ctx context.Context, principal auth.Principal, input CreateInput) (Batch, bool, error) {
	if err := s.authorize(ctx, principal); err != nil {
		return Batch{}, false, err
	}
	if !normalizeCreateInput(&input) {
		return Batch{}, false, ErrInvalidInput
	}
	requestHash, err := hashRequest(input)
	if err != nil {
		return Batch{}, false, ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Batch{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var batchID string
	err = tx.QueryRow(ctx, `INSERT INTO batches(company_id,marketplace_key,created_by,idempotency_key,request_hash) VALUES($1,$2,$3,$4,$5) ON CONFLICT(company_id,idempotency_key) DO NOTHING RETURNING id`, principal.CompanyID, input.MarketplaceKey, principal.UserID, input.IdempotencyKey, requestHash).Scan(&batchID)
	if errors.Is(err, pgx.ErrNoRows) {
		var existingHash string
		if err = tx.QueryRow(ctx, `SELECT id,request_hash FROM batches WHERE company_id=$1 AND idempotency_key=$2`, principal.CompanyID, input.IdempotencyKey).Scan(&batchID, &existingHash); err != nil {
			return Batch{}, false, err
		}
		if existingHash != requestHash {
			return Batch{}, false, ErrConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return Batch{}, false, err
		}
		item, getErr := s.get(ctx, principal.CompanyID, batchID)
		return item, true, getErr
	}
	if err != nil {
		return Batch{}, false, err
	}
	var eligibleCount int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM marketplace_orders mo JOIN processing_jobs pj ON pj.company_id=mo.company_id AND pj.id=mo.processing_job_id WHERE mo.company_id=$1 AND mo.marketplace_key=$2 AND mo.id=ANY($3::uuid[]) AND pj.status IN ('processed','needs_review') AND mo.status<>'duplicate'`, principal.CompanyID, input.MarketplaceKey, input.OrderIDs).Scan(&eligibleCount); err != nil {
		return Batch{}, false, err
	}
	if eligibleCount != len(input.OrderIDs) {
		return Batch{}, false, ErrIneligible
	}
	for index, orderID := range input.OrderIDs {
		if _, err = tx.Exec(ctx, `INSERT INTO batch_members(company_id,batch_id,marketplace_order_id,position) VALUES($1,$2,$3,$4)`, principal.CompanyID, batchID, orderID, index+1); err != nil {
			return Batch{}, false, mapDBError(err)
		}
	}
	if err = s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, "batch.created", "batch", batchID, map[string]any{"marketplace": input.MarketplaceKey, "order_count": len(input.OrderIDs)}); err != nil {
		return Batch{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Batch{}, false, err
	}
	item, err := s.get(ctx, principal.CompanyID, batchID)
	return item, false, err
}

func (s *Service) Get(ctx context.Context, principal auth.Principal, id string) (Batch, error) {
	if err := s.authorize(ctx, principal); err != nil {
		return Batch{}, err
	}
	return s.get(ctx, principal.CompanyID, id)
}

func (s *Service) get(ctx context.Context, companyID, id string) (Batch, error) {
	var item Batch
	err := scanBatch(s.db.QueryRow(ctx, `
		SELECT b.id,b.marketplace_key,b.status,b.created_by,count(DISTINCT bm.marketplace_order_id),
		       count(*) FILTER (WHERE mo.id IS NOT NULL AND (mo.status<>'resolved' OR moi.id IS NULL OR moi.product_id IS NULL OR moi.quantity IS NULL OR moi.resolution_status<>'resolved')),
		       b.ready_at,b.cancelled_at,b.created_at,b.updated_at
		FROM batches b
		LEFT JOIN batch_members bm ON bm.company_id=b.company_id AND bm.batch_id=b.id
		LEFT JOIN marketplace_orders mo ON mo.company_id=bm.company_id AND mo.id=bm.marketplace_order_id
		LEFT JOIN marketplace_order_items moi ON moi.company_id=mo.company_id AND moi.order_id=mo.id
		WHERE b.company_id=$1 AND b.id=$2 GROUP BY b.id`, companyID, id), &item)
	if err != nil {
		return Batch{}, mapDBError(err)
	}
	item.Members, err = s.members(ctx, companyID, id)
	if err != nil {
		return Batch{}, err
	}
	item.ProductTotals, err = s.productTotals(ctx, companyID, id)
	return item, err
}

func (s *Service) Ready(ctx context.Context, principal auth.Principal, id string) (Batch, error) {
	return s.transition(ctx, principal, id, "ready")
}

func (s *Service) Cancel(ctx context.Context, principal auth.Principal, id string) (Batch, error) {
	return s.transition(ctx, principal, id, "cancelled")
}

func (s *Service) transition(ctx context.Context, principal auth.Principal, id, target string) (Batch, error) {
	if err := s.authorize(ctx, principal); err != nil {
		return Batch{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Batch{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var status string
	if err = tx.QueryRow(ctx, `SELECT status FROM batches WHERE company_id=$1 AND id=$2 FOR UPDATE`, principal.CompanyID, id).Scan(&status); err != nil {
		return Batch{}, mapDBError(err)
	}
	if status == target {
		if err = tx.Commit(ctx); err != nil {
			return Batch{}, err
		}
		return s.get(ctx, principal.CompanyID, id)
	}
	if status != "draft" || (target != "ready" && target != "cancelled") {
		return Batch{}, ErrInvalidState
	}
	if target == "ready" {
		var unresolved int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM batch_members bm JOIN marketplace_orders mo ON mo.company_id=bm.company_id AND mo.id=bm.marketplace_order_id LEFT JOIN marketplace_order_items moi ON moi.company_id=mo.company_id AND moi.order_id=mo.id WHERE bm.company_id=$1 AND bm.batch_id=$2 AND (mo.status<>'resolved' OR moi.id IS NULL OR moi.product_id IS NULL OR moi.quantity IS NULL OR moi.resolution_status<>'resolved')`, principal.CompanyID, id).Scan(&unresolved); err != nil {
			return Batch{}, err
		}
		if unresolved > 0 {
			return Batch{}, ErrUnresolved
		}
	}
	timestampColumn, action := "ready_at", "batch.ready"
	if target == "cancelled" {
		timestampColumn, action = "cancelled_at", "batch.cancelled"
	}
	if _, err = tx.Exec(ctx, `UPDATE batches SET status=$1,`+timestampColumn+`=now(),updated_at=now() WHERE company_id=$2 AND id=$3`, target, principal.CompanyID, id); err != nil {
		return Batch{}, err
	}
	if err = s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, action, "batch", id, map[string]any{"previous_status": status, "status": target}); err != nil {
		return Batch{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Batch{}, err
	}
	return s.get(ctx, principal.CompanyID, id)
}

func (s *Service) authorize(ctx context.Context, principal auth.Principal) error {
	if err := s.authorizer.RequireModule(ctx, principal, "flipkart"); err != nil {
		return err
	}
	return s.authorizer.RequirePermission(ctx, principal, "labels.process")
}

func (s *Service) members(ctx context.Context, companyID, batchID string) ([]Member, error) {
	rows, err := s.db.Query(ctx, `SELECT mo.id,bm.position,mo.source_file_id,mo.processing_job_id,mo.source_page,mo.marketplace_order_id,mo.awb,mo.status FROM batch_members bm JOIN marketplace_orders mo ON mo.company_id=bm.company_id AND mo.id=bm.marketplace_order_id WHERE bm.company_id=$1 AND bm.batch_id=$2 ORDER BY bm.position`, companyID, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Member, 0)
	for rows.Next() {
		var item Member
		if err := rows.Scan(&item.OrderID, &item.Position, &item.SourceFileID, &item.ProcessingJobID, &item.SourcePage, &item.MarketplaceOrderID, &item.AWB, &item.Status); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) productTotals(ctx context.Context, companyID, batchID string) ([]ProductTotal, error) {
	rows, err := s.db.Query(ctx, `SELECT p.id,p.internal_code,p.name,sum(moi.quantity)::integer,count(moi.id)::integer FROM batch_members bm JOIN marketplace_order_items moi ON moi.company_id=bm.company_id AND moi.order_id=bm.marketplace_order_id JOIN products p ON p.company_id=moi.company_id AND p.id=moi.product_id WHERE bm.company_id=$1 AND bm.batch_id=$2 AND moi.quantity IS NOT NULL AND moi.resolution_status='resolved' GROUP BY p.id ORDER BY p.internal_code,p.id`, companyID, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ProductTotal, 0)
	for rows.Next() {
		var item ProductTotal
		if err := rows.Scan(&item.ProductID, &item.InternalCode, &item.ProductName, &item.TotalQuantity, &item.OrderLineCount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanBatch(row interface{ Scan(...any) error }, item *Batch) error {
	return row.Scan(&item.ID, &item.MarketplaceKey, &item.Status, &item.CreatedBy, &item.OrderCount, &item.UnresolvedCount, &item.ReadyAt, &item.CancelledAt, &item.CreatedAt, &item.UpdatedAt)
}

func normalizeCreateInput(input *CreateInput) bool {
	input.MarketplaceKey = strings.TrimSpace(input.MarketplaceKey)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.MarketplaceKey != "flipkart" || input.IdempotencyKey == "" || len(input.IdempotencyKey) > 128 || len(input.OrderIDs) == 0 || len(input.OrderIDs) > maxBatchMembers {
		return false
	}
	seen := make(map[string]struct{}, len(input.OrderIDs))
	for index := range input.OrderIDs {
		input.OrderIDs[index] = strings.TrimSpace(input.OrderIDs[index])
		if !uuidRE.MatchString(input.OrderIDs[index]) {
			return false
		}
		if _, exists := seen[input.OrderIDs[index]]; exists {
			return false
		}
		seen[input.OrderIDs[index]] = struct{}{}
	}
	return true
}

func hashRequest(input CreateInput) (string, error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func mapDBError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	return err
}
