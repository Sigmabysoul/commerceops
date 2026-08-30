package inventory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/jackc/pgx/v5"
)

type OutboundInput struct {
	IdempotencyKey string `json:"idempotency_key"`
}

func (s *Service) ConfirmEcommerceOutbound(ctx context.Context, p auth.Principal, batchID string, input OutboundInput) ([]Transaction, bool, error) {
	if err := s.authorize(ctx, p, "inventory.dispatch"); err != nil {
		return nil, false, err
	}
	batchID, input.IdempotencyKey = strings.TrimSpace(batchID), strings.TrimSpace(input.IdempotencyKey)
	if !uuidRE.MatchString(batchID) || input.IdempotencyKey == "" || len(input.IdempotencyKey) > 128 {
		return nil, false, ErrInvalidInput
	}
	h := sha256.Sum256([]byte("ecommerce_out\x00" + batchID + "\x00" + input.IdempotencyKey))
	requestHash := hex.EncodeToString(h[:])
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var eventID string
	err = tx.QueryRow(ctx, `INSERT INTO inventory_outbound_events(company_id,batch_id,actor_user_id,idempotency_key,request_hash) SELECT $1,id,$2,$3,$4 FROM batches WHERE company_id=$1 AND id=$5 AND status='ready' ON CONFLICT DO NOTHING RETURNING id`, p.CompanyID, p.UserID, input.IdempotencyKey, requestHash, batchID).Scan(&eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		var oldBatch, oldHash string
		if qerr := tx.QueryRow(ctx, `SELECT batch_id,request_hash FROM inventory_outbound_events WHERE company_id=$1 AND (batch_id=$2 OR idempotency_key=$3)`, p.CompanyID, batchID, input.IdempotencyKey).Scan(&oldBatch, &oldHash); qerr != nil {
			return nil, false, mapDBError(qerr)
		}
		if oldBatch != batchID || oldHash != requestHash {
			return nil, false, ErrConflict
		}
		items, qerr := transactionsForReference(ctx, tx, p.CompanyID, "batch", batchID)
		if qerr != nil {
			return nil, false, qerr
		}
		if err = tx.Commit(ctx); err != nil {
			return nil, false, err
		}
		return items, true, nil
	}
	if err != nil {
		return nil, false, mapDBError(err)
	}
	rows, err := tx.Query(ctx, `SELECT moi.product_id,sum(moi.quantity)::bigint FROM batch_members bm JOIN marketplace_order_items moi ON moi.company_id=bm.company_id AND moi.order_id=bm.marketplace_order_id WHERE bm.company_id=$1 AND bm.batch_id=$2 AND moi.product_id IS NOT NULL AND moi.quantity>0 AND moi.resolution_status='resolved' GROUP BY moi.product_id ORDER BY moi.product_id`, p.CompanyID, batchID)
	if err != nil {
		return nil, false, err
	}
	type total struct {
		id       string
		quantity int64
	}
	totals := []total{}
	for rows.Next() {
		var v total
		if err = rows.Scan(&v.id, &v.quantity); err != nil {
			rows.Close()
			return nil, false, err
		}
		totals = append(totals, v)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	if len(totals) == 0 {
		return nil, false, ErrInvalidInput
	}
	for _, v := range totals {
		if _, err = tx.Exec(ctx, `INSERT INTO inventory_balances(company_id,product_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, p.CompanyID, v.id); err != nil {
			return nil, false, err
		}
	}
	items := make([]Transaction, 0, len(totals))
	for _, v := range totals {
		var previous, reserved int64
		if err = tx.QueryRow(ctx, `SELECT on_hand,reserved FROM inventory_balances WHERE company_id=$1 AND product_id=$2 FOR UPDATE`, p.CompanyID, v.id).Scan(&previous, &reserved); err != nil {
			return nil, false, err
		}
		result := previous - v.quantity
		if result < 0 || result < reserved {
			return nil, false, ErrInsufficientStock
		}
		keyHash := sha256.Sum256([]byte(input.IdempotencyKey + "\x00" + v.id))
		key := hex.EncodeToString(keyHash[:])
		reason := "Confirmed ecommerce batch outbound"
		var item Transaction
		err = scanTransaction(tx.QueryRow(ctx, `INSERT INTO inventory_transactions(company_id,product_id,transaction_type,quantity_delta,previous_balance,resulting_balance,reason,reference_type,reference_id,actor_user_id,idempotency_key,request_hash) VALUES($1,$2,'ecommerce_out',$3,$4,$5,$6,'batch',$7,$8,$9,$10) RETURNING id,product_id,transaction_type,quantity_delta,previous_balance,resulting_balance,reason,reference_type,reference_id,actor_user_id,created_at`, p.CompanyID, v.id, -v.quantity, previous, result, reason, batchID, p.UserID, key, requestHash), &item)
		if err != nil {
			return nil, false, mapDBError(err)
		}
		if _, err = tx.Exec(ctx, `UPDATE inventory_balances SET on_hand=$1,updated_at=now() WHERE company_id=$2 AND product_id=$3`, result, p.CompanyID, v.id); err != nil {
			return nil, false, err
		}
		items = append(items, item)
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "inventory.ecommerce_out", "batch", batchID, map[string]any{"product_count": len(items), "event_id": eventID}); err != nil {
		return nil, false, err
	}
	return items, false, tx.Commit(ctx)
}

func transactionsForReference(ctx context.Context, tx pgx.Tx, companyID, kind, id string) ([]Transaction, error) {
	rows, err := tx.Query(ctx, `SELECT id,product_id,transaction_type,quantity_delta,previous_balance,resulting_balance,reason,reference_type,reference_id,actor_user_id,created_at FROM inventory_transactions WHERE company_id=$1 AND transaction_type='ecommerce_out' AND reference_type=$2 AND reference_id=$3 ORDER BY product_id`, companyID, kind, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Transaction{}
	for rows.Next() {
		var i Transaction
		if err = scanTransaction(rows, &i); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}
