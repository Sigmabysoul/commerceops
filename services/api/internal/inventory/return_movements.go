package inventory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/jackc/pgx/v5"
)

type ReturnMovement struct {
	ProductID string `json:"product_id"`
	Quantity  int64  `json:"quantity"`
}

func (s *Service) ApplyReturnRestock(ctx context.Context, tx pgx.Tx, p auth.Principal, returnID string, movements []ReturnMovement) ([]Transaction, error) {
	return s.applyReturnMovements(ctx, tx, p, "return_restock", "return_case", returnID, returnID, movements)
}

func (s *Service) ApplyReturnRestockCorrection(ctx context.Context, tx pgx.Tx, p auth.Principal, returnID, correctionEventID string, movements []ReturnMovement) ([]Transaction, error) {
	return s.applyReturnMovements(ctx, tx, p, "correction", "return_restock_correction", correctionEventID, returnID, movements)
}

func (s *Service) applyReturnMovements(ctx context.Context, tx pgx.Tx, p auth.Principal, transactionType, referenceType, referenceID, returnID string, movements []ReturnMovement) ([]Transaction, error) {
	if err := s.authorizer.RequireModule(ctx, p, "inventory"); err != nil {
		return nil, err
	}
	if err := s.authorizer.RequirePermission(ctx, p, "returns.restock"); err != nil {
		return nil, err
	}
	if tx == nil || !uuidRE.MatchString(referenceID) || !uuidRE.MatchString(returnID) || len(movements) == 0 || len(movements) > 100 {
		return nil, ErrInvalidInput
	}
	seen := make(map[string]bool, len(movements))
	for _, movement := range movements {
		if !uuidRE.MatchString(movement.ProductID) || movement.Quantity <= 0 || seen[movement.ProductID] {
			return nil, ErrInvalidInput
		}
		seen[movement.ProductID] = true
	}
	sort.Slice(movements, func(i, j int) bool { return movements[i].ProductID < movements[j].ProductID })
	payload, _ := json.Marshal(struct {
		TransactionType string           `json:"transaction_type"`
		ReferenceType   string           `json:"reference_type"`
		ReferenceID     string           `json:"reference_id"`
		ReturnID        string           `json:"return_id"`
		Movements       []ReturnMovement `json:"movements"`
	}{transactionType, referenceType, referenceID, returnID, movements})
	digest := sha256.Sum256(payload)
	requestHash := hex.EncodeToString(digest[:])

	for _, movement := range movements {
		if _, err := tx.Exec(ctx, `INSERT INTO inventory_balances(company_id,product_id) SELECT $1,id FROM products WHERE company_id=$1 AND id=$2 ON CONFLICT DO NOTHING`, p.CompanyID, movement.ProductID); err != nil {
			return nil, mapDBError(err)
		}
	}
	items := make([]Transaction, 0, len(movements))
	for _, movement := range movements {
		var previous, reserved int64
		if err := tx.QueryRow(ctx, `SELECT on_hand,reserved FROM inventory_balances WHERE company_id=$1 AND product_id=$2 FOR UPDATE`, p.CompanyID, movement.ProductID).Scan(&previous, &reserved); err != nil {
			return nil, mapDBError(err)
		}
		delta := movement.Quantity
		reason := "Accepted physical return for restock"
		if transactionType == "correction" {
			delta = -movement.Quantity
			reason = "Compensating correction for return restock"
		}
		result := previous + delta
		if result < 0 || result < reserved {
			return nil, ErrInsufficientStock
		}
		keyDigest := sha256.Sum256([]byte(transactionType + "\x00" + referenceID + "\x00" + movement.ProductID))
		idempotencyKey := hex.EncodeToString(keyDigest[:])
		var item Transaction
		err := scanTransaction(tx.QueryRow(ctx, `INSERT INTO inventory_transactions(company_id,product_id,transaction_type,quantity_delta,previous_balance,resulting_balance,reason,reference_type,reference_id,actor_user_id,idempotency_key,request_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id,product_id,transaction_type,quantity_delta,previous_balance,resulting_balance,reason,reference_type,reference_id,actor_user_id,created_at`, p.CompanyID, movement.ProductID, transactionType, delta, previous, result, reason, referenceType, referenceID, p.UserID, idempotencyKey, requestHash), &item)
		if err != nil {
			return nil, mapDBError(err)
		}
		if _, err = tx.Exec(ctx, `UPDATE inventory_balances SET on_hand=$1,updated_at=now() WHERE company_id=$2 AND product_id=$3`, result, p.CompanyID, movement.ProductID); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	action := "inventory.return_restock"
	if transactionType == "correction" {
		action = "inventory.return_restock_correction"
	}
	if err := s.audit.Record(ctx, tx, p.CompanyID, p.UserID, action, "return_case", returnID, map[string]any{"reference_id": referenceID, "product_count": len(items)}); err != nil {
		return nil, err
	}
	return items, nil
}
