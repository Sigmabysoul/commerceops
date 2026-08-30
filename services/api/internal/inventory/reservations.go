package inventory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/jackc/pgx/v5"
)

type Reservation struct {
	ID            string     `json:"id"`
	ProductID     string     `json:"product_id"`
	Quantity      int64      `json:"quantity"`
	Status        string     `json:"status"`
	Reason        string     `json:"reason"`
	SourceType    string     `json:"source_type"`
	SourceID      string     `json:"source_id"`
	CreatedBy     string     `json:"created_by"`
	ReleasedBy    *string    `json:"released_by"`
	ReleaseReason *string    `json:"release_reason"`
	CreatedAt     time.Time  `json:"created_at"`
	ReleasedAt    *time.Time `json:"released_at"`
}
type ReserveInput struct {
	ProductID      string `json:"product_id"`
	Quantity       int64  `json:"quantity"`
	Reason         string `json:"reason"`
	SourceType     string `json:"source_type"`
	SourceID       string `json:"source_id"`
	IdempotencyKey string `json:"idempotency_key"`
}
type ReleaseInput struct {
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key"`
}

func (s *Service) ListReservations(ctx context.Context, p auth.Principal, status string) ([]Reservation, error) {
	if err := s.authorize(ctx, p, "inventory.view"); err != nil {
		return nil, err
	}
	status = strings.TrimSpace(status)
	if status != "" && status != "active" && status != "released" {
		return nil, ErrInvalidInput
	}
	rows, err := s.db.Query(ctx, `SELECT id,product_id,quantity,status,reason,source_type,source_id,created_by,released_by,release_reason,created_at,released_at FROM inventory_reservations WHERE company_id=$1 AND ($2='' OR status=$2) ORDER BY created_at DESC,id DESC LIMIT 500`, p.CompanyID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Reservation{}
	for rows.Next() {
		var i Reservation
		if err = scanReservation(rows, &i); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}
func (s *Service) Reserve(ctx context.Context, p auth.Principal, i ReserveInput) (Reservation, bool, error) {
	if err := s.authorize(ctx, p, "inventory.adjust"); err != nil {
		return Reservation{}, false, err
	}
	i.ProductID = strings.TrimSpace(i.ProductID)
	i.Reason = strings.TrimSpace(i.Reason)
	i.SourceType = strings.TrimSpace(i.SourceType)
	i.SourceID = strings.TrimSpace(i.SourceID)
	i.IdempotencyKey = strings.TrimSpace(i.IdempotencyKey)
	if !uuidRE.MatchString(i.ProductID) || i.Quantity <= 0 || i.Reason == "" || len(i.Reason) > 500 || i.SourceType == "" || len(i.SourceType) > 100 || i.SourceID == "" || len(i.SourceID) > 200 || i.IdempotencyKey == "" || len(i.IdempotencyKey) > 128 {
		return Reservation{}, false, ErrInvalidInput
	}
	hash := hashJSON(i)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Reservation{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err = tx.Exec(ctx, `INSERT INTO inventory_balances(company_id,product_id) SELECT company_id,id FROM products WHERE company_id=$1 AND id=$2 ON CONFLICT DO NOTHING`, p.CompanyID, i.ProductID); err != nil {
		return Reservation{}, false, err
	}
	var onHand, reserved int64
	if err = tx.QueryRow(ctx, `SELECT on_hand,reserved FROM inventory_balances WHERE company_id=$1 AND product_id=$2 FOR UPDATE`, p.CompanyID, i.ProductID).Scan(&onHand, &reserved); err != nil {
		return Reservation{}, false, mapDBError(err)
	}
	var item Reservation
	var oldHash string
	err = scanReservationHash(tx.QueryRow(ctx, `SELECT id,product_id,quantity,status,reason,source_type,source_id,created_by,released_by,release_reason,created_at,released_at,create_request_hash FROM inventory_reservations WHERE company_id=$1 AND (create_idempotency_key=$2 OR (source_type=$3 AND source_id=$4 AND product_id=$5))`, p.CompanyID, i.IdempotencyKey, i.SourceType, i.SourceID, i.ProductID), &item, &oldHash)
	if err == nil {
		if oldHash != hash {
			return Reservation{}, false, ErrConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return Reservation{}, false, err
		}
		return item, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Reservation{}, false, err
	}
	if reserved+i.Quantity > onHand {
		return Reservation{}, false, ErrInsufficientStock
	}
	err = scanReservation(tx.QueryRow(ctx, `INSERT INTO inventory_reservations(company_id,product_id,quantity,reason,source_type,source_id,created_by,create_idempotency_key,create_request_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id,product_id,quantity,status,reason,source_type,source_id,created_by,released_by,release_reason,created_at,released_at`, p.CompanyID, i.ProductID, i.Quantity, i.Reason, i.SourceType, i.SourceID, p.UserID, i.IdempotencyKey, hash), &item)
	if err != nil {
		return Reservation{}, false, mapDBError(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE inventory_balances SET reserved=reserved+$1,updated_at=now() WHERE company_id=$2 AND product_id=$3`, i.Quantity, p.CompanyID, i.ProductID); err != nil {
		return Reservation{}, false, err
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "inventory.reserved", "inventory_reservation", item.ID, map[string]any{"quantity": i.Quantity, "product_id": i.ProductID}); err != nil {
		return Reservation{}, false, err
	}
	return item, false, tx.Commit(ctx)
}
func (s *Service) ReleaseReservation(ctx context.Context, p auth.Principal, id string, i ReleaseInput) (Reservation, bool, error) {
	if err := s.authorize(ctx, p, "inventory.adjust"); err != nil {
		return Reservation{}, false, err
	}
	id = strings.TrimSpace(id)
	i.Reason = strings.TrimSpace(i.Reason)
	i.IdempotencyKey = strings.TrimSpace(i.IdempotencyKey)
	if !uuidRE.MatchString(id) || i.Reason == "" || len(i.Reason) > 500 || i.IdempotencyKey == "" || len(i.IdempotencyKey) > 128 {
		return Reservation{}, false, ErrInvalidInput
	}
	hash := hashJSON(struct {
		ID    string
		Input ReleaseInput
	}{id, i})
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Reservation{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var item Reservation
	var releaseHash *string
	err = scanReservationReleaseHash(tx.QueryRow(ctx, `SELECT id,product_id,quantity,status,reason,source_type,source_id,created_by,released_by,release_reason,created_at,released_at,release_request_hash FROM inventory_reservations WHERE company_id=$1 AND id=$2 FOR UPDATE`, p.CompanyID, id), &item, &releaseHash)
	if err != nil {
		return Reservation{}, false, mapDBError(err)
	}
	if item.Status == "released" {
		if releaseHash == nil || *releaseHash != hash {
			return Reservation{}, false, ErrConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return Reservation{}, false, err
		}
		return item, true, nil
	}
	if _, err = tx.Exec(ctx, `UPDATE inventory_balances SET reserved=reserved-$1,updated_at=now() WHERE company_id=$2 AND product_id=$3`, item.Quantity, p.CompanyID, item.ProductID); err != nil {
		return Reservation{}, false, err
	}
	err = scanReservation(tx.QueryRow(ctx, `UPDATE inventory_reservations SET status='released',released_by=$1,release_reason=$2,release_idempotency_key=$3,release_request_hash=$4,released_at=now() WHERE company_id=$5 AND id=$6 RETURNING id,product_id,quantity,status,reason,source_type,source_id,created_by,released_by,release_reason,created_at,released_at`, p.UserID, i.Reason, i.IdempotencyKey, hash, p.CompanyID, id), &item)
	if err != nil {
		return Reservation{}, false, mapDBError(err)
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "inventory.reservation_released", "inventory_reservation", item.ID, map[string]any{"quantity": item.Quantity, "product_id": item.ProductID, "reason": i.Reason}); err != nil {
		return Reservation{}, false, err
	}
	return item, false, tx.Commit(ctx)
}
func hashJSON(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func scanReservation(r scanner, i *Reservation) error {
	return r.Scan(&i.ID, &i.ProductID, &i.Quantity, &i.Status, &i.Reason, &i.SourceType, &i.SourceID, &i.CreatedBy, &i.ReleasedBy, &i.ReleaseReason, &i.CreatedAt, &i.ReleasedAt)
}
func scanReservationHash(r scanner, i *Reservation, h *string) error {
	return r.Scan(&i.ID, &i.ProductID, &i.Quantity, &i.Status, &i.Reason, &i.SourceType, &i.SourceID, &i.CreatedBy, &i.ReleasedBy, &i.ReleaseReason, &i.CreatedAt, &i.ReleasedAt, h)
}
func scanReservationReleaseHash(r scanner, i *Reservation, h **string) error {
	return r.Scan(&i.ID, &i.ProductID, &i.Quantity, &i.Status, &i.Reason, &i.SourceType, &i.SourceID, &i.CreatedBy, &i.ReleasedBy, &i.ReleaseReason, &i.CreatedAt, &i.ReleasedAt, h)
}
