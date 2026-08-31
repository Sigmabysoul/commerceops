package returns

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
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
	ErrNotFound       = errors.New("return or cancellation not found")
	ErrInvalidInput   = errors.New("invalid return or cancellation input")
	ErrConflict       = errors.New("return or cancellation conflict")
	ErrInvalidState   = errors.New("return state transition is not allowed")
	ErrQuantity       = errors.New("return quantity exceeds the order quantity")
	uuidRE            = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89aAbB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
	marketplaceRE     = regexp.MustCompile(`^[a-z][a-z0-9_]{0,49}$`)
	validReturnStatus = map[string]bool{"expected": true, "received": true, "inspection_pending": true, "restocked": true, "damaged": true, "rejected": true, "closed": true}
)

type Service struct {
	db         *pgxpool.Pool
	authorizer *authorization.Service
	audit      audit.Recorder
}

type Cancellation struct {
	ID                 string    `json:"id"`
	MarketplaceOrderID string    `json:"marketplace_order_id"`
	Marketplace        string    `json:"marketplace"`
	ExternalOrderID    *string   `json:"external_order_id"`
	Status             string    `json:"status"`
	OutboundState      string    `json:"outbound_state"`
	Reason             string    `json:"reason"`
	CancelledAt        time.Time `json:"cancelled_at"`
	RecordedBy         string    `json:"recorded_by"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type CreateCancellationInput struct {
	MarketplaceOrderID string    `json:"marketplace_order_id"`
	Reason             string    `json:"reason"`
	CancelledAt        time.Time `json:"cancelled_at"`
	IdempotencyKey     string    `json:"idempotency_key"`
}

type ReturnCase struct {
	ID                 string        `json:"id"`
	MarketplaceOrderID string        `json:"marketplace_order_id"`
	Marketplace        string        `json:"marketplace"`
	ExternalOrderID    *string       `json:"external_order_id"`
	Status             string        `json:"status"`
	Reason             string        `json:"reason"`
	Notes              *string       `json:"notes"`
	CreatedBy          string        `json:"created_by"`
	ReceivedBy         *string       `json:"received_by"`
	ReceivedAt         *time.Time    `json:"received_at"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
	Items              []ReturnItem  `json:"items"`
	Events             []ReturnEvent `json:"events"`
}

type ReturnItem struct {
	ID                     string `json:"id"`
	MarketplaceOrderItemID string `json:"marketplace_order_item_id"`
	ProductID              string `json:"product_id"`
	InternalCode           string `json:"internal_code"`
	ProductName            string `json:"product_name"`
	ExpectedQuantity       int64  `json:"expected_quantity"`
	ReceivedQuantity       *int64 `json:"received_quantity"`
	Disposition            string `json:"disposition"`
}

type ReturnEvent struct {
	ID          string    `json:"id"`
	EventType   string    `json:"event_type"`
	ActorUserID string    `json:"actor_user_id"`
	Notes       *string   `json:"notes"`
	CreatedAt   time.Time `json:"created_at"`
}

type ExpectedItemInput struct {
	MarketplaceOrderItemID string `json:"marketplace_order_item_id"`
	ExpectedQuantity       int64  `json:"expected_quantity"`
}

type CreateReturnInput struct {
	MarketplaceOrderID string              `json:"marketplace_order_id"`
	Reason             string              `json:"reason"`
	Notes              *string             `json:"notes"`
	Items              []ExpectedItemInput `json:"items"`
	IdempotencyKey     string              `json:"idempotency_key"`
}

type ReceivedItemInput struct {
	ReturnItemID     string `json:"return_item_id"`
	ReceivedQuantity int64  `json:"received_quantity"`
}

type ReceiveReturnInput struct {
	Items          []ReceivedItemInput `json:"items"`
	Notes          *string             `json:"notes"`
	IdempotencyKey string              `json:"idempotency_key"`
}

func NewService(db *pgxpool.Pool, authorizer *authorization.Service) *Service {
	return &Service{db: db, authorizer: authorizer}
}

func (s *Service) ListCancellations(ctx context.Context, p auth.Principal, status, marketplace string) ([]Cancellation, error) {
	if err := s.authorize(ctx, p, "returns.view"); err != nil {
		return nil, err
	}
	status, marketplace = strings.TrimSpace(status), strings.TrimSpace(marketplace)
	if status != "" && status != "recorded" && status != "closed" || marketplace != "" && !marketplaceRE.MatchString(marketplace) {
		return nil, ErrInvalidInput
	}
	rows, err := s.db.Query(ctx, cancellationSelect+` WHERE c.company_id=$1 AND ($2='' OR c.status=$2) AND ($3='' OR o.marketplace_key=$3) ORDER BY c.created_at DESC,c.id DESC LIMIT 500`, p.CompanyID, status, marketplace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Cancellation, 0)
	for rows.Next() {
		var item Cancellation
		if err = scanCancellation(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) GetCancellation(ctx context.Context, p auth.Principal, id string) (Cancellation, error) {
	if err := s.authorize(ctx, p, "returns.view"); err != nil {
		return Cancellation{}, err
	}
	id = strings.TrimSpace(id)
	if !uuidRE.MatchString(id) {
		return Cancellation{}, ErrInvalidInput
	}
	return loadCancellation(s.db.QueryRow(ctx, cancellationSelect+` WHERE c.company_id=$1 AND c.id=$2`, p.CompanyID, id))
}

func (s *Service) CreateCancellation(ctx context.Context, p auth.Principal, input CreateCancellationInput) (Cancellation, bool, error) {
	if err := s.authorize(ctx, p, "returns.manage"); err != nil {
		return Cancellation{}, false, err
	}
	input.MarketplaceOrderID = strings.TrimSpace(input.MarketplaceOrderID)
	input.Reason = strings.TrimSpace(input.Reason)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if !uuidRE.MatchString(input.MarketplaceOrderID) || input.Reason == "" || len(input.Reason) > 500 || input.CancelledAt.IsZero() || input.IdempotencyKey == "" || len(input.IdempotencyKey) > 128 {
		return Cancellation{}, false, ErrInvalidInput
	}
	hash := requestHash(input)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Cancellation{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var existing Cancellation
	var oldHash string
	err = scanCancellationHash(tx.QueryRow(ctx, `SELECT c.id,c.marketplace_order_id,o.marketplace_key,o.marketplace_order_id,c.status,c.outbound_state,c.reason,c.cancelled_at,c.recorded_by,c.created_at,c.updated_at,c.request_hash FROM cancellations c JOIN marketplace_orders o ON o.company_id=c.company_id AND o.id=c.marketplace_order_id WHERE c.company_id=$1 AND c.idempotency_key=$2`, p.CompanyID, input.IdempotencyKey), &existing, &oldHash)
	if err == nil {
		if oldHash != hash {
			return Cancellation{}, false, ErrConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return Cancellation{}, false, err
		}
		return existing, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Cancellation{}, false, err
	}
	var marketplace string
	err = tx.QueryRow(ctx, `SELECT marketplace_key FROM marketplace_orders WHERE company_id=$1 AND id=$2 AND status='resolved' FOR UPDATE`, p.CompanyID, input.MarketplaceOrderID).Scan(&marketplace)
	if errors.Is(err, pgx.ErrNoRows) {
		return Cancellation{}, false, ErrNotFound
	}
	if err != nil {
		return Cancellation{}, false, err
	}
	err = scanCancellationHash(tx.QueryRow(ctx, `SELECT c.id,c.marketplace_order_id,o.marketplace_key,o.marketplace_order_id,c.status,c.outbound_state,c.reason,c.cancelled_at,c.recorded_by,c.created_at,c.updated_at,c.request_hash FROM cancellations c JOIN marketplace_orders o ON o.company_id=c.company_id AND o.id=c.marketplace_order_id WHERE c.company_id=$1 AND c.idempotency_key=$2`, p.CompanyID, input.IdempotencyKey), &existing, &oldHash)
	if err == nil {
		if oldHash != hash {
			return Cancellation{}, false, ErrConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return Cancellation{}, false, err
		}
		return existing, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Cancellation{}, false, err
	}
	var outbound bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM batch_members bm JOIN inventory_outbound_events e ON e.company_id=bm.company_id AND e.batch_id=bm.batch_id WHERE bm.company_id=$1 AND bm.marketplace_order_id=$2)`, p.CompanyID, input.MarketplaceOrderID).Scan(&outbound)
	if err != nil {
		return Cancellation{}, false, err
	}
	outboundState := "not_outbound"
	if outbound {
		outboundState = "outbound_confirmed"
	}
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO cancellations(company_id,marketplace_order_id,outbound_state,reason,cancelled_at,recorded_by,idempotency_key,request_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, p.CompanyID, input.MarketplaceOrderID, outboundState, input.Reason, input.CancelledAt, p.UserID, input.IdempotencyKey, hash).Scan(&id)
	if err != nil {
		return Cancellation{}, false, mapDBError(err)
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "returns.cancellation_recorded", "cancellation", id, map[string]any{"marketplace": marketplace, "marketplace_order_id": input.MarketplaceOrderID, "outbound_state": outboundState}); err != nil {
		return Cancellation{}, false, err
	}
	created, err := loadCancellation(tx.QueryRow(ctx, cancellationSelect+` WHERE c.company_id=$1 AND c.id=$2`, p.CompanyID, id))
	if err != nil {
		return Cancellation{}, false, err
	}
	return created, false, tx.Commit(ctx)
}

func (s *Service) ListReturns(ctx context.Context, p auth.Principal, status, marketplace string) ([]ReturnCase, error) {
	if err := s.authorize(ctx, p, "returns.view"); err != nil {
		return nil, err
	}
	status, marketplace = strings.TrimSpace(status), strings.TrimSpace(marketplace)
	if status != "" && !validReturnStatus[status] || marketplace != "" && !marketplaceRE.MatchString(marketplace) {
		return nil, ErrInvalidInput
	}
	rows, err := s.db.Query(ctx, returnSelect+` WHERE r.company_id=$1 AND ($2='' OR r.status=$2) AND ($3='' OR o.marketplace_key=$3) ORDER BY r.created_at DESC,r.id DESC LIMIT 500`, p.CompanyID, status, marketplace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ReturnCase, 0)
	for rows.Next() {
		var item ReturnCase
		if err = scanReturn(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) GetReturn(ctx context.Context, p auth.Principal, id string) (ReturnCase, error) {
	if err := s.authorize(ctx, p, "returns.view"); err != nil {
		return ReturnCase{}, err
	}
	id = strings.TrimSpace(id)
	if !uuidRE.MatchString(id) {
		return ReturnCase{}, ErrInvalidInput
	}
	return loadReturn(ctx, s.db, p.CompanyID, id)
}

func (s *Service) CreateReturn(ctx context.Context, p auth.Principal, input CreateReturnInput) (ReturnCase, bool, error) {
	if err := s.authorize(ctx, p, "returns.manage"); err != nil {
		return ReturnCase{}, false, err
	}
	input.MarketplaceOrderID = strings.TrimSpace(input.MarketplaceOrderID)
	input.Reason = strings.TrimSpace(input.Reason)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.Notes = trimOptional(input.Notes)
	if !uuidRE.MatchString(input.MarketplaceOrderID) || input.Reason == "" || len(input.Reason) > 500 || input.IdempotencyKey == "" || len(input.IdempotencyKey) > 128 || len(input.Items) == 0 || len(input.Items) > 100 || input.Notes != nil && len(*input.Notes) > 2000 {
		return ReturnCase{}, false, ErrInvalidInput
	}
	seen := make(map[string]bool, len(input.Items))
	for index := range input.Items {
		input.Items[index].MarketplaceOrderItemID = strings.TrimSpace(input.Items[index].MarketplaceOrderItemID)
		if !uuidRE.MatchString(input.Items[index].MarketplaceOrderItemID) || input.Items[index].ExpectedQuantity <= 0 || seen[input.Items[index].MarketplaceOrderItemID] {
			return ReturnCase{}, false, ErrInvalidInput
		}
		seen[input.Items[index].MarketplaceOrderItemID] = true
	}
	sort.Slice(input.Items, func(i, j int) bool {
		return input.Items[i].MarketplaceOrderItemID < input.Items[j].MarketplaceOrderItemID
	})
	hash := requestHash(input)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ReturnCase{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var existingID, oldHash string
	err = tx.QueryRow(ctx, `SELECT id,request_hash FROM return_cases WHERE company_id=$1 AND idempotency_key=$2`, p.CompanyID, input.IdempotencyKey).Scan(&existingID, &oldHash)
	if err == nil {
		if oldHash != hash {
			return ReturnCase{}, false, ErrConflict
		}
		existing, loadErr := loadReturn(ctx, tx, p.CompanyID, existingID)
		if loadErr != nil {
			return ReturnCase{}, false, loadErr
		}
		if err = tx.Commit(ctx); err != nil {
			return ReturnCase{}, false, err
		}
		return existing, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ReturnCase{}, false, err
	}
	var marketplace string
	err = tx.QueryRow(ctx, `SELECT marketplace_key FROM marketplace_orders WHERE company_id=$1 AND id=$2 AND status='resolved' FOR UPDATE`, p.CompanyID, input.MarketplaceOrderID).Scan(&marketplace)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReturnCase{}, false, ErrNotFound
	}
	if err != nil {
		return ReturnCase{}, false, err
	}
	err = tx.QueryRow(ctx, `SELECT id,request_hash FROM return_cases WHERE company_id=$1 AND idempotency_key=$2`, p.CompanyID, input.IdempotencyKey).Scan(&existingID, &oldHash)
	if err == nil {
		if oldHash != hash {
			return ReturnCase{}, false, ErrConflict
		}
		existing, loadErr := loadReturn(ctx, tx, p.CompanyID, existingID)
		if loadErr != nil {
			return ReturnCase{}, false, loadErr
		}
		if err = tx.Commit(ctx); err != nil {
			return ReturnCase{}, false, err
		}
		return existing, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ReturnCase{}, false, err
	}
	type resolvedItem struct {
		orderItemID string
		productID   string
		expected    int64
	}
	resolved := make([]resolvedItem, 0, len(input.Items))
	for _, requested := range input.Items {
		var productID string
		var ordered, alreadyExpected int64
		err = tx.QueryRow(ctx, `SELECT oi.product_id,oi.quantity FROM marketplace_order_items oi WHERE oi.company_id=$1 AND oi.id=$2 AND oi.order_id=$3 AND oi.resolution_status='resolved' AND oi.product_id IS NOT NULL AND oi.quantity IS NOT NULL FOR UPDATE`, p.CompanyID, requested.MarketplaceOrderItemID, input.MarketplaceOrderID).Scan(&productID, &ordered)
		if errors.Is(err, pgx.ErrNoRows) {
			return ReturnCase{}, false, ErrNotFound
		}
		if err != nil {
			return ReturnCase{}, false, err
		}
		if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(expected_quantity),0) FROM return_items WHERE company_id=$1 AND marketplace_order_item_id=$2`, p.CompanyID, requested.MarketplaceOrderItemID).Scan(&alreadyExpected); err != nil {
			return ReturnCase{}, false, err
		}
		if alreadyExpected+requested.ExpectedQuantity > ordered {
			return ReturnCase{}, false, ErrQuantity
		}
		resolved = append(resolved, resolvedItem{requested.MarketplaceOrderItemID, productID, requested.ExpectedQuantity})
	}
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO return_cases(company_id,marketplace_order_id,reason,notes,created_by,idempotency_key,request_hash) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, p.CompanyID, input.MarketplaceOrderID, input.Reason, input.Notes, p.UserID, input.IdempotencyKey, hash).Scan(&id)
	if err != nil {
		return ReturnCase{}, false, mapDBError(err)
	}
	for _, item := range resolved {
		if _, err = tx.Exec(ctx, `INSERT INTO return_items(company_id,return_case_id,marketplace_order_item_id,product_id,expected_quantity) VALUES($1,$2,$3,$4,$5)`, p.CompanyID, id, item.orderItemID, item.productID, item.expected); err != nil {
			return ReturnCase{}, false, mapDBError(err)
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO return_events(company_id,return_case_id,event_type,actor_user_id,notes,idempotency_key,request_hash) VALUES($1,$2,'created',$3,$4,$5,$6)`, p.CompanyID, id, p.UserID, input.Notes, input.IdempotencyKey, hash); err != nil {
		return ReturnCase{}, false, mapDBError(err)
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "returns.created", "return_case", id, map[string]any{"marketplace": marketplace, "marketplace_order_id": input.MarketplaceOrderID, "item_count": len(resolved)}); err != nil {
		return ReturnCase{}, false, err
	}
	created, err := loadReturn(ctx, tx, p.CompanyID, id)
	if err != nil {
		return ReturnCase{}, false, err
	}
	return created, false, tx.Commit(ctx)
}

func (s *Service) ReceiveReturn(ctx context.Context, p auth.Principal, id string, input ReceiveReturnInput) (ReturnCase, bool, error) {
	if err := s.authorize(ctx, p, "returns.manage"); err != nil {
		return ReturnCase{}, false, err
	}
	id = strings.TrimSpace(id)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.Notes = trimOptional(input.Notes)
	if !uuidRE.MatchString(id) || input.IdempotencyKey == "" || len(input.IdempotencyKey) > 128 || len(input.Items) == 0 || len(input.Items) > 100 || input.Notes != nil && len(*input.Notes) > 2000 {
		return ReturnCase{}, false, ErrInvalidInput
	}
	seen := make(map[string]bool, len(input.Items))
	for index := range input.Items {
		input.Items[index].ReturnItemID = strings.TrimSpace(input.Items[index].ReturnItemID)
		if !uuidRE.MatchString(input.Items[index].ReturnItemID) || input.Items[index].ReceivedQuantity < 0 || seen[input.Items[index].ReturnItemID] {
			return ReturnCase{}, false, ErrInvalidInput
		}
		seen[input.Items[index].ReturnItemID] = true
	}
	sort.Slice(input.Items, func(i, j int) bool { return input.Items[i].ReturnItemID < input.Items[j].ReturnItemID })
	hash := requestHash(struct {
		ReturnID string             `json:"return_id"`
		Input    ReceiveReturnInput `json:"input"`
	}{id, input})
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ReturnCase{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var existingReturnID, oldHash string
	err = tx.QueryRow(ctx, `SELECT return_case_id,request_hash FROM return_events WHERE company_id=$1 AND idempotency_key=$2`, p.CompanyID, input.IdempotencyKey).Scan(&existingReturnID, &oldHash)
	if err == nil {
		if existingReturnID != id || oldHash != hash {
			return ReturnCase{}, false, ErrConflict
		}
		existing, loadErr := loadReturn(ctx, tx, p.CompanyID, id)
		if loadErr != nil {
			return ReturnCase{}, false, loadErr
		}
		if err = tx.Commit(ctx); err != nil {
			return ReturnCase{}, false, err
		}
		return existing, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ReturnCase{}, false, err
	}
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM return_cases WHERE company_id=$1 AND id=$2 FOR UPDATE`, p.CompanyID, id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReturnCase{}, false, ErrNotFound
	}
	if err != nil {
		return ReturnCase{}, false, err
	}
	if status != "expected" {
		err = tx.QueryRow(ctx, `SELECT return_case_id,request_hash FROM return_events WHERE company_id=$1 AND idempotency_key=$2`, p.CompanyID, input.IdempotencyKey).Scan(&existingReturnID, &oldHash)
		if err == nil && existingReturnID == id && oldHash == hash {
			existing, loadErr := loadReturn(ctx, tx, p.CompanyID, id)
			if loadErr != nil {
				return ReturnCase{}, false, loadErr
			}
			if err = tx.Commit(ctx); err != nil {
				return ReturnCase{}, false, err
			}
			return existing, true, nil
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return ReturnCase{}, false, err
		}
		return ReturnCase{}, false, ErrInvalidState
	}
	rows, err := tx.Query(ctx, `SELECT id,expected_quantity FROM return_items WHERE company_id=$1 AND return_case_id=$2 ORDER BY id FOR UPDATE`, p.CompanyID, id)
	if err != nil {
		return ReturnCase{}, false, err
	}
	expected := make(map[string]int64)
	for rows.Next() {
		var itemID string
		var quantity int64
		if err = rows.Scan(&itemID, &quantity); err != nil {
			rows.Close()
			return ReturnCase{}, false, err
		}
		expected[itemID] = quantity
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return ReturnCase{}, false, err
	}
	rows.Close()
	if len(expected) != len(input.Items) {
		return ReturnCase{}, false, ErrInvalidInput
	}
	for _, received := range input.Items {
		max, ok := expected[received.ReturnItemID]
		if !ok {
			return ReturnCase{}, false, ErrNotFound
		}
		if received.ReceivedQuantity > max {
			return ReturnCase{}, false, ErrQuantity
		}
		if _, err = tx.Exec(ctx, `UPDATE return_items SET received_quantity=$1,updated_at=now() WHERE company_id=$2 AND return_case_id=$3 AND id=$4`, received.ReceivedQuantity, p.CompanyID, id, received.ReturnItemID); err != nil {
			return ReturnCase{}, false, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE return_cases SET status='received',received_by=$1,received_at=now(),updated_at=now() WHERE company_id=$2 AND id=$3`, p.UserID, p.CompanyID, id); err != nil {
		return ReturnCase{}, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO return_events(company_id,return_case_id,event_type,actor_user_id,notes,idempotency_key,request_hash) VALUES($1,$2,'received',$3,$4,$5,$6)`, p.CompanyID, id, p.UserID, input.Notes, input.IdempotencyKey, hash); err != nil {
		return ReturnCase{}, false, mapDBError(err)
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "returns.received", "return_case", id, map[string]any{"item_count": len(input.Items)}); err != nil {
		return ReturnCase{}, false, err
	}
	updated, err := loadReturn(ctx, tx, p.CompanyID, id)
	if err != nil {
		return ReturnCase{}, false, err
	}
	return updated, false, tx.Commit(ctx)
}

func (s *Service) authorize(ctx context.Context, p auth.Principal, permission string) error {
	if err := s.authorizer.RequireModule(ctx, p, "returns"); err != nil {
		return err
	}
	return s.authorizer.RequirePermission(ctx, p, permission)
}

const cancellationSelect = `SELECT c.id,c.marketplace_order_id,o.marketplace_key,o.marketplace_order_id,c.status,c.outbound_state,c.reason,c.cancelled_at,c.recorded_by,c.created_at,c.updated_at FROM cancellations c JOIN marketplace_orders o ON o.company_id=c.company_id AND o.id=c.marketplace_order_id`
const returnSelect = `SELECT r.id,r.marketplace_order_id,o.marketplace_key,o.marketplace_order_id,r.status,r.reason,r.notes,r.created_by,r.received_by,r.received_at,r.created_at,r.updated_at FROM return_cases r JOIN marketplace_orders o ON o.company_id=r.company_id AND o.id=r.marketplace_order_id`

type scanner interface{ Scan(...any) error }
type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func scanCancellation(row scanner, item *Cancellation) error {
	return row.Scan(&item.ID, &item.MarketplaceOrderID, &item.Marketplace, &item.ExternalOrderID, &item.Status, &item.OutboundState, &item.Reason, &item.CancelledAt, &item.RecordedBy, &item.CreatedAt, &item.UpdatedAt)
}

func loadCancellation(row scanner) (Cancellation, error) {
	var item Cancellation
	if err := scanCancellation(row, &item); err != nil {
		return Cancellation{}, mapDBError(err)
	}
	return item, nil
}

func scanCancellationHash(row scanner, item *Cancellation, hash *string) error {
	return row.Scan(&item.ID, &item.MarketplaceOrderID, &item.Marketplace, &item.ExternalOrderID, &item.Status, &item.OutboundState, &item.Reason, &item.CancelledAt, &item.RecordedBy, &item.CreatedAt, &item.UpdatedAt, hash)
}

func scanReturn(row scanner, item *ReturnCase) error {
	item.Items = make([]ReturnItem, 0)
	item.Events = make([]ReturnEvent, 0)
	return row.Scan(&item.ID, &item.MarketplaceOrderID, &item.Marketplace, &item.ExternalOrderID, &item.Status, &item.Reason, &item.Notes, &item.CreatedBy, &item.ReceivedBy, &item.ReceivedAt, &item.CreatedAt, &item.UpdatedAt)
}

func loadReturn(ctx context.Context, q queryer, companyID, id string) (ReturnCase, error) {
	var item ReturnCase
	if err := scanReturn(q.QueryRow(ctx, returnSelect+` WHERE r.company_id=$1 AND r.id=$2`, companyID, id), &item); err != nil {
		return ReturnCase{}, mapDBError(err)
	}
	rows, err := q.Query(ctx, `SELECT ri.id,ri.marketplace_order_item_id,ri.product_id,p.internal_code,p.name,ri.expected_quantity,ri.received_quantity,ri.disposition FROM return_items ri JOIN products p ON p.company_id=ri.company_id AND p.id=ri.product_id WHERE ri.company_id=$1 AND ri.return_case_id=$2 ORDER BY ri.created_at,ri.id`, companyID, id)
	if err != nil {
		return ReturnCase{}, err
	}
	for rows.Next() {
		var child ReturnItem
		if err = rows.Scan(&child.ID, &child.MarketplaceOrderItemID, &child.ProductID, &child.InternalCode, &child.ProductName, &child.ExpectedQuantity, &child.ReceivedQuantity, &child.Disposition); err != nil {
			rows.Close()
			return ReturnCase{}, err
		}
		item.Items = append(item.Items, child)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return ReturnCase{}, err
	}
	rows.Close()
	rows, err = q.Query(ctx, `SELECT id,event_type,actor_user_id,notes,created_at FROM return_events WHERE company_id=$1 AND return_case_id=$2 ORDER BY created_at,id`, companyID, id)
	if err != nil {
		return ReturnCase{}, err
	}
	for rows.Next() {
		var event ReturnEvent
		if err = rows.Scan(&event.ID, &event.EventType, &event.ActorUserID, &event.Notes, &event.CreatedAt); err != nil {
			rows.Close()
			return ReturnCase{}, err
		}
		item.Events = append(item.Events, event)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return ReturnCase{}, err
	}
	rows.Close()
	return item, nil
}

func requestHash(value any) string {
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func trimOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
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
		switch pgErr.Code {
		case "23505":
			return ErrConflict
		case "23503", "23514", "22P02":
			return ErrInvalidInput
		}
	}
	return err
}
