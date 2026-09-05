// This file coordinates the package's business rules and persistence operations behind a reusable API in the inventory package.
package inventory

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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound          = errors.New("inventory product not found")
	ErrInvalidInput      = errors.New("invalid inventory input")
	ErrConflict          = errors.New("inventory idempotency conflict")
	ErrInsufficientStock = errors.New("inventory cannot become negative")
	ErrCancelledOrder    = errors.New("batch contains a cancelled order")
	uuidRE               = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89aAbB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
)

type Service struct {
	db         *pgxpool.Pool
	authorizer *authorization.Service
	audit      audit.Recorder
}
type Balance struct {
	ProductID    string    `json:"product_id"`
	InternalCode string    `json:"internal_code"`
	ProductName  string    `json:"product_name"`
	OnHand       int64     `json:"on_hand"`
	Reserved     int64     `json:"reserved"`
	Available    int64     `json:"available"`
	UpdatedAt    time.Time `json:"updated_at"`
}
type Transaction struct {
	ID               string    `json:"id"`
	ProductID        string    `json:"product_id"`
	TransactionType  string    `json:"transaction_type"`
	QuantityDelta    int64     `json:"quantity_delta"`
	PreviousBalance  int64     `json:"previous_balance"`
	ResultingBalance int64     `json:"resulting_balance"`
	Reason           string    `json:"reason"`
	ReferenceType    *string   `json:"reference_type"`
	ReferenceID      *string   `json:"reference_id"`
	ActorUserID      string    `json:"actor_user_id"`
	CreatedAt        time.Time `json:"created_at"`
}
type CommandInput struct {
	ProductID      string  `json:"product_id"`
	Quantity       int64   `json:"quantity"`
	Reason         string  `json:"reason"`
	ReferenceType  *string `json:"reference_type"`
	ReferenceID    *string `json:"reference_id"`
	IdempotencyKey string  `json:"idempotency_key"`
}

func NewService(db *pgxpool.Pool, a *authorization.Service) *Service {
	return &Service{db: db, authorizer: a}
}

func (s *Service) ListBalances(ctx context.Context, p auth.Principal) ([]Balance, error) {
	if err := s.authorize(ctx, p, "inventory.view"); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT p.id,p.internal_code,p.name,COALESCE(b.on_hand,0),COALESCE(b.reserved,0),COALESCE(b.on_hand,0)-COALESCE(b.reserved,0),COALESCE(b.updated_at,p.updated_at) FROM products p LEFT JOIN inventory_balances b ON b.company_id=p.company_id AND b.product_id=p.id WHERE p.company_id=$1 ORDER BY p.name,p.internal_code,p.id LIMIT 500`, p.CompanyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Balance, 0)
	for rows.Next() {
		var item Balance
		if err = rows.Scan(&item.ProductID, &item.InternalCode, &item.ProductName, &item.OnHand, &item.Reserved, &item.Available, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func (s *Service) ListTransactions(ctx context.Context, p auth.Principal, productID, kind string) ([]Transaction, error) {
	if err := s.authorize(ctx, p, "inventory.view"); err != nil {
		return nil, err
	}
	productID, kind = strings.TrimSpace(productID), strings.TrimSpace(kind)
	if productID != "" && !uuidRE.MatchString(productID) || kind != "" && !validType(kind) {
		return nil, ErrInvalidInput
	}
	rows, err := s.db.Query(ctx, `SELECT id,product_id,transaction_type,quantity_delta,previous_balance,resulting_balance,reason,reference_type,reference_id,actor_user_id,created_at FROM inventory_transactions WHERE company_id=$1 AND ($2='' OR product_id::text=$2) AND ($3='' OR transaction_type=$3) ORDER BY created_at DESC,id DESC LIMIT 500`, p.CompanyID, productID, kind)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()
	items := make([]Transaction, 0)
	for rows.Next() {
		var item Transaction
		if err = scanTransaction(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func (s *Service) StockIn(ctx context.Context, p auth.Principal, i CommandInput) (Transaction, bool, error) {
	if i.Quantity <= 0 {
		return Transaction{}, false, ErrInvalidInput
	}
	return s.mutate(ctx, p, "inventory.stock_in", "stock_in", i)
}
func (s *Service) Adjust(ctx context.Context, p auth.Principal, i CommandInput) (Transaction, bool, error) {
	if i.Quantity == 0 {
		return Transaction{}, false, ErrInvalidInput
	}
	return s.mutate(ctx, p, "inventory.adjust", "manual_adjustment", i)
}
func (s *Service) Correct(ctx context.Context, p auth.Principal, i CommandInput) (Transaction, bool, error) {
	if i.Quantity == 0 {
		return Transaction{}, false, ErrInvalidInput
	}
	return s.mutate(ctx, p, "inventory.adjust", "correction", i)
}

func (s *Service) mutate(ctx context.Context, p auth.Principal, permission, kind string, i CommandInput) (Transaction, bool, error) {
	if err := s.authorize(ctx, p, permission); err != nil {
		return Transaction{}, false, err
	}
	i.ProductID = strings.TrimSpace(i.ProductID)
	i.Reason = strings.TrimSpace(i.Reason)
	i.IdempotencyKey = strings.TrimSpace(i.IdempotencyKey)
	i.ReferenceType = trim(i.ReferenceType)
	i.ReferenceID = trim(i.ReferenceID)
	if !uuidRE.MatchString(i.ProductID) || i.Reason == "" || len(i.Reason) > 500 || i.IdempotencyKey == "" || len(i.IdempotencyKey) > 128 || (i.ReferenceType == nil) != (i.ReferenceID == nil) {
		return Transaction{}, false, ErrInvalidInput
	}
	payload, _ := json.Marshal(struct {
		Type  string       `json:"type"`
		Input CommandInput `json:"input"`
	}{kind, i})
	digest := sha256.Sum256(payload)
	hash := hex.EncodeToString(digest[:])
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Transaction{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM products WHERE company_id=$1 AND id=$2)`, p.CompanyID, i.ProductID).Scan(&exists); err != nil {
		return Transaction{}, false, err
	}
	if !exists {
		return Transaction{}, false, ErrNotFound
	}
	if _, err = tx.Exec(ctx, `INSERT INTO inventory_balances(company_id,product_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, p.CompanyID, i.ProductID); err != nil {
		return Transaction{}, false, mapDBError(err)
	}
	var previous, reserved int64
	if err = tx.QueryRow(ctx, `SELECT on_hand,reserved FROM inventory_balances WHERE company_id=$1 AND product_id=$2 FOR UPDATE`, p.CompanyID, i.ProductID).Scan(&previous, &reserved); err != nil {
		return Transaction{}, false, mapDBError(err)
	}
	var existing Transaction
	var oldHash string
	err = scanTransactionHash(tx.QueryRow(ctx, `SELECT id,product_id,transaction_type,quantity_delta,previous_balance,resulting_balance,reason,reference_type,reference_id,actor_user_id,created_at,request_hash FROM inventory_transactions WHERE company_id=$1 AND idempotency_key=$2`, p.CompanyID, i.IdempotencyKey), &existing, &oldHash)
	if err == nil {
		if oldHash != hash {
			return Transaction{}, false, ErrConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return Transaction{}, false, err
		}
		return existing, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Transaction{}, false, err
	}
	result := previous + i.Quantity
	if result < 0 || result < reserved {
		return Transaction{}, false, ErrInsufficientStock
	}
	var item Transaction
	err = scanTransaction(tx.QueryRow(ctx, `INSERT INTO inventory_transactions(company_id,product_id,transaction_type,quantity_delta,previous_balance,resulting_balance,reason,reference_type,reference_id,actor_user_id,idempotency_key,request_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id,product_id,transaction_type,quantity_delta,previous_balance,resulting_balance,reason,reference_type,reference_id,actor_user_id,created_at`, p.CompanyID, i.ProductID, kind, i.Quantity, previous, result, i.Reason, i.ReferenceType, i.ReferenceID, p.UserID, i.IdempotencyKey, hash), &item)
	if err != nil {
		return Transaction{}, false, mapDBError(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE inventory_balances SET on_hand=$1,updated_at=now() WHERE company_id=$2 AND product_id=$3`, result, p.CompanyID, i.ProductID); err != nil {
		return Transaction{}, false, err
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "inventory."+kind, "inventory_transaction", item.ID, map[string]any{"product_id": i.ProductID, "quantity_delta": i.Quantity, "previous_balance": previous, "resulting_balance": result}); err != nil {
		return Transaction{}, false, err
	}
	return item, false, tx.Commit(ctx)
}
func (s *Service) authorize(ctx context.Context, p auth.Principal, permission string) error {
	if err := s.authorizer.RequireModule(ctx, p, "inventory"); err != nil {
		return err
	}
	return s.authorizer.RequirePermission(ctx, p, permission)
}
func validType(v string) bool {
	return v == "stock_in" || v == "manual_adjustment" || v == "correction" || v == "ecommerce_out" || v == "return_restock" || v == "consignment_out"
}
func trim(v *string) *string {
	if v == nil {
		return nil
	}
	x := strings.TrimSpace(*v)
	if x == "" {
		return nil
	}
	return &x
}

type scanner interface{ Scan(...any) error }

func scanTransaction(r scanner, i *Transaction) error {
	return r.Scan(&i.ID, &i.ProductID, &i.TransactionType, &i.QuantityDelta, &i.PreviousBalance, &i.ResultingBalance, &i.Reason, &i.ReferenceType, &i.ReferenceID, &i.ActorUserID, &i.CreatedAt)
}
func scanTransactionHash(r scanner, i *Transaction, h *string) error {
	return r.Scan(&i.ID, &i.ProductID, &i.TransactionType, &i.QuantityDelta, &i.PreviousBalance, &i.ResultingBalance, &i.Reason, &i.ReferenceType, &i.ReferenceID, &i.ActorUserID, &i.CreatedAt, h)
}
func mapDBError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var e *pgconn.PgError
	if errors.As(err, &e) {
		if e.Code == "23505" {
			return ErrConflict
		}
		if e.Code == "23503" || e.Code == "23514" || e.Code == "22P02" {
			return ErrInvalidInput
		}
	}
	return err
}
