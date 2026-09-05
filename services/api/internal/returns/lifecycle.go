// This file implements allowed return-state transitions and their domain side effects in the returns package.
package returns

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/inventory"
	"github.com/jackc/pgx/v5"
)

type DispositionItemInput struct {
	ReturnItemID string `json:"return_item_id"`
	Disposition  string `json:"disposition"`
}

type InspectReturnInput struct {
	Items          []DispositionItemInput `json:"items"`
	Notes          *string                `json:"notes"`
	IdempotencyKey string                 `json:"idempotency_key"`
}

type RestockReturnInput struct {
	Notes          *string `json:"notes"`
	IdempotencyKey string  `json:"idempotency_key"`
}

type CorrectionItemInput struct {
	ReturnItemID string `json:"return_item_id"`
	Quantity     int64  `json:"quantity"`
}

type CorrectRestockInput struct {
	Items          []CorrectionItemInput `json:"items"`
	Reason         string                `json:"reason"`
	IdempotencyKey string                `json:"idempotency_key"`
}

func (s *Service) InspectReturn(ctx context.Context, p auth.Principal, id string, input InspectReturnInput) (ReturnCase, bool, error) {
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
		input.Items[index].Disposition = strings.TrimSpace(input.Items[index].Disposition)
		if !uuidRE.MatchString(input.Items[index].ReturnItemID) || !validDisposition(input.Items[index].Disposition) || seen[input.Items[index].ReturnItemID] {
			return ReturnCase{}, false, ErrInvalidInput
		}
		seen[input.Items[index].ReturnItemID] = true
	}
	sort.Slice(input.Items, func(i, j int) bool { return input.Items[i].ReturnItemID < input.Items[j].ReturnItemID })
	hash := lifecycleHash(id, input)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ReturnCase{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if replay, replayErr := eventReplay(ctx, tx, p.CompanyID, id, input.IdempotencyKey, hash); replayErr != nil {
		return ReturnCase{}, false, replayErr
	} else if replay {
		return commitReplay(ctx, tx, p.CompanyID, id)
	}
	var status string
	if err = tx.QueryRow(ctx, `SELECT status FROM return_cases WHERE company_id=$1 AND id=$2 FOR UPDATE`, p.CompanyID, id).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return ReturnCase{}, false, ErrNotFound
	} else if err != nil {
		return ReturnCase{}, false, err
	}
	if replay, replayErr := eventReplay(ctx, tx, p.CompanyID, id, input.IdempotencyKey, hash); replayErr != nil {
		return ReturnCase{}, false, replayErr
	} else if replay {
		return commitReplay(ctx, tx, p.CompanyID, id)
	}
	if status != "received" {
		return ReturnCase{}, false, ErrInvalidState
	}
	rows, err := tx.Query(ctx, `SELECT id,received_quantity FROM return_items WHERE company_id=$1 AND return_case_id=$2 ORDER BY id FOR UPDATE`, p.CompanyID, id)
	if err != nil {
		return ReturnCase{}, false, err
	}
	received := make(map[string]int64)
	for rows.Next() {
		var itemID string
		var quantity *int64
		if err = rows.Scan(&itemID, &quantity); err != nil {
			rows.Close()
			return ReturnCase{}, false, err
		}
		if quantity == nil {
			rows.Close()
			return ReturnCase{}, false, ErrInvalidState
		}
		received[itemID] = *quantity
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return ReturnCase{}, false, err
	}
	rows.Close()
	if len(received) != len(input.Items) {
		return ReturnCase{}, false, ErrInvalidInput
	}
	anyRestockable, anyDamaged := false, false
	for _, inspected := range input.Items {
		quantity, ok := received[inspected.ReturnItemID]
		if !ok {
			return ReturnCase{}, false, ErrNotFound
		}
		if quantity == 0 && inspected.Disposition != "missing" || quantity > 0 && inspected.Disposition == "missing" {
			return ReturnCase{}, false, ErrInvalidInput
		}
		anyRestockable = anyRestockable || inspected.Disposition == "restockable"
		anyDamaged = anyDamaged || inspected.Disposition == "damaged"
		if _, err = tx.Exec(ctx, `UPDATE return_items SET disposition=$1,updated_at=now() WHERE company_id=$2 AND return_case_id=$3 AND id=$4`, inspected.Disposition, p.CompanyID, id, inspected.ReturnItemID); err != nil {
			return ReturnCase{}, false, err
		}
	}
	nextStatus := "rejected"
	if anyRestockable {
		nextStatus = "inspected"
	} else if anyDamaged {
		nextStatus = "damaged"
	}
	if _, err = tx.Exec(ctx, `UPDATE return_cases SET status=$1,updated_at=now() WHERE company_id=$2 AND id=$3`, nextStatus, p.CompanyID, id); err != nil {
		return ReturnCase{}, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO return_events(company_id,return_case_id,event_type,actor_user_id,notes,idempotency_key,request_hash) VALUES($1,$2,'inspected',$3,$4,$5,$6)`, p.CompanyID, id, p.UserID, input.Notes, input.IdempotencyKey, hash); err != nil {
		return ReturnCase{}, false, mapDBError(err)
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "returns.inspected", "return_case", id, map[string]any{"status": nextStatus, "item_count": len(input.Items)}); err != nil {
		return ReturnCase{}, false, err
	}
	return commitResult(ctx, tx, p.CompanyID, id, false)
}

func (s *Service) RestockReturn(ctx context.Context, p auth.Principal, id string, input RestockReturnInput) (ReturnCase, bool, error) {
	if err := s.authorize(ctx, p, "returns.restock"); err != nil {
		return ReturnCase{}, false, err
	}
	if s.inventory == nil {
		return ReturnCase{}, false, ErrInventoryState
	}
	id = strings.TrimSpace(id)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.Notes = trimOptional(input.Notes)
	if !uuidRE.MatchString(id) || input.IdempotencyKey == "" || len(input.IdempotencyKey) > 128 || input.Notes != nil && len(*input.Notes) > 2000 {
		return ReturnCase{}, false, ErrInvalidInput
	}
	hash := lifecycleHash(id, input)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ReturnCase{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if replay, replayErr := eventReplay(ctx, tx, p.CompanyID, id, input.IdempotencyKey, hash); replayErr != nil {
		return ReturnCase{}, false, replayErr
	} else if replay {
		return commitReplay(ctx, tx, p.CompanyID, id)
	}
	var status string
	if err = tx.QueryRow(ctx, `SELECT status FROM return_cases WHERE company_id=$1 AND id=$2 FOR UPDATE`, p.CompanyID, id).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return ReturnCase{}, false, ErrNotFound
	} else if err != nil {
		return ReturnCase{}, false, err
	}
	if replay, replayErr := eventReplay(ctx, tx, p.CompanyID, id, input.IdempotencyKey, hash); replayErr != nil {
		return ReturnCase{}, false, replayErr
	} else if replay {
		return commitReplay(ctx, tx, p.CompanyID, id)
	}
	if status != "inspected" {
		return ReturnCase{}, false, ErrInvalidState
	}
	rows, err := tx.Query(ctx, `SELECT id,product_id,received_quantity,disposition FROM return_items WHERE company_id=$1 AND return_case_id=$2 ORDER BY product_id,id FOR UPDATE`, p.CompanyID, id)
	if err != nil {
		return ReturnCase{}, false, err
	}
	totals := make(map[string]int64)
	itemQuantities := make(map[string]int64)
	for rows.Next() {
		var itemID, productID, disposition string
		var quantity *int64
		if err = rows.Scan(&itemID, &productID, &quantity, &disposition); err != nil {
			rows.Close()
			return ReturnCase{}, false, err
		}
		if disposition == "restockable" && quantity != nil && *quantity > 0 {
			totals[productID] += *quantity
			itemQuantities[itemID] = *quantity
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return ReturnCase{}, false, err
	}
	rows.Close()
	if len(totals) == 0 {
		return ReturnCase{}, false, ErrInvalidState
	}
	movements := make([]inventory.ReturnMovement, 0, len(totals))
	for productID, quantity := range totals {
		movements = append(movements, inventory.ReturnMovement{ProductID: productID, Quantity: quantity})
	}
	var eventID string
	if err = tx.QueryRow(ctx, `INSERT INTO return_events(company_id,return_case_id,event_type,actor_user_id,notes,idempotency_key,request_hash) VALUES($1,$2,'restocked',$3,$4,$5,$6) RETURNING id`, p.CompanyID, id, p.UserID, input.Notes, input.IdempotencyKey, hash).Scan(&eventID); err != nil {
		return ReturnCase{}, false, mapDBError(err)
	}
	if _, err = s.inventory.ApplyReturnRestock(ctx, tx, p, id, movements); err != nil {
		return ReturnCase{}, false, mapInventoryError(err)
	}
	for itemID, quantity := range itemQuantities {
		if _, err = tx.Exec(ctx, `UPDATE return_items SET restocked_quantity=$1,updated_at=now() WHERE company_id=$2 AND return_case_id=$3 AND id=$4`, quantity, p.CompanyID, id, itemID); err != nil {
			return ReturnCase{}, false, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE return_cases SET status='restocked',updated_at=now() WHERE company_id=$1 AND id=$2`, p.CompanyID, id); err != nil {
		return ReturnCase{}, false, err
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "returns.restocked", "return_case", id, map[string]any{"event_id": eventID, "product_count": len(movements)}); err != nil {
		return ReturnCase{}, false, err
	}
	return commitResult(ctx, tx, p.CompanyID, id, false)
}

func (s *Service) CorrectRestock(ctx context.Context, p auth.Principal, id string, input CorrectRestockInput) (ReturnCase, bool, error) {
	if err := s.authorize(ctx, p, "returns.restock"); err != nil {
		return ReturnCase{}, false, err
	}
	if s.inventory == nil {
		return ReturnCase{}, false, ErrInventoryState
	}
	id = strings.TrimSpace(id)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.Reason = strings.TrimSpace(input.Reason)
	if !uuidRE.MatchString(id) || input.IdempotencyKey == "" || len(input.IdempotencyKey) > 128 || input.Reason == "" || len(input.Reason) > 500 || len(input.Items) == 0 || len(input.Items) > 100 {
		return ReturnCase{}, false, ErrInvalidInput
	}
	seen := make(map[string]bool, len(input.Items))
	for index := range input.Items {
		input.Items[index].ReturnItemID = strings.TrimSpace(input.Items[index].ReturnItemID)
		if !uuidRE.MatchString(input.Items[index].ReturnItemID) || input.Items[index].Quantity <= 0 || seen[input.Items[index].ReturnItemID] {
			return ReturnCase{}, false, ErrInvalidInput
		}
		seen[input.Items[index].ReturnItemID] = true
	}
	sort.Slice(input.Items, func(i, j int) bool { return input.Items[i].ReturnItemID < input.Items[j].ReturnItemID })
	hash := lifecycleHash(id, input)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ReturnCase{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if replay, replayErr := eventReplay(ctx, tx, p.CompanyID, id, input.IdempotencyKey, hash); replayErr != nil {
		return ReturnCase{}, false, replayErr
	} else if replay {
		return commitReplay(ctx, tx, p.CompanyID, id)
	}
	var status string
	if err = tx.QueryRow(ctx, `SELECT status FROM return_cases WHERE company_id=$1 AND id=$2 FOR UPDATE`, p.CompanyID, id).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return ReturnCase{}, false, ErrNotFound
	} else if err != nil {
		return ReturnCase{}, false, err
	}
	if replay, replayErr := eventReplay(ctx, tx, p.CompanyID, id, input.IdempotencyKey, hash); replayErr != nil {
		return ReturnCase{}, false, replayErr
	} else if replay {
		return commitReplay(ctx, tx, p.CompanyID, id)
	}
	if status != "restocked" && status != "restock_corrected" {
		return ReturnCase{}, false, ErrInvalidState
	}
	type correctable struct {
		productID string
		remaining int64
	}
	correctableItems := make(map[string]correctable, len(input.Items))
	rows, err := tx.Query(ctx, `SELECT id,product_id,restocked_quantity-corrected_quantity FROM return_items WHERE company_id=$1 AND return_case_id=$2 ORDER BY id FOR UPDATE`, p.CompanyID, id)
	if err != nil {
		return ReturnCase{}, false, err
	}
	for rows.Next() {
		var itemID string
		var item correctable
		if err = rows.Scan(&itemID, &item.productID, &item.remaining); err != nil {
			rows.Close()
			return ReturnCase{}, false, err
		}
		correctableItems[itemID] = item
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return ReturnCase{}, false, err
	}
	rows.Close()
	totals := make(map[string]int64)
	for _, correction := range input.Items {
		item, ok := correctableItems[correction.ReturnItemID]
		if !ok {
			return ReturnCase{}, false, ErrNotFound
		}
		if correction.Quantity > item.remaining {
			return ReturnCase{}, false, ErrQuantity
		}
		totals[item.productID] += correction.Quantity
	}
	movements := make([]inventory.ReturnMovement, 0, len(totals))
	for productID, quantity := range totals {
		movements = append(movements, inventory.ReturnMovement{ProductID: productID, Quantity: quantity})
	}
	var eventID string
	if err = tx.QueryRow(ctx, `INSERT INTO return_events(company_id,return_case_id,event_type,actor_user_id,notes,idempotency_key,request_hash) VALUES($1,$2,'restock_corrected',$3,$4,$5,$6) RETURNING id`, p.CompanyID, id, p.UserID, input.Reason, input.IdempotencyKey, hash).Scan(&eventID); err != nil {
		return ReturnCase{}, false, mapDBError(err)
	}
	if _, err = s.inventory.ApplyReturnRestockCorrection(ctx, tx, p, id, eventID, movements); err != nil {
		return ReturnCase{}, false, mapInventoryError(err)
	}
	for _, correction := range input.Items {
		if _, err = tx.Exec(ctx, `UPDATE return_items SET corrected_quantity=corrected_quantity+$1,updated_at=now() WHERE company_id=$2 AND return_case_id=$3 AND id=$4`, correction.Quantity, p.CompanyID, id, correction.ReturnItemID); err != nil {
			return ReturnCase{}, false, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE return_cases SET status='restock_corrected',updated_at=now() WHERE company_id=$1 AND id=$2`, p.CompanyID, id); err != nil {
		return ReturnCase{}, false, err
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "returns.restock_corrected", "return_case", id, map[string]any{"event_id": eventID, "product_count": len(movements)}); err != nil {
		return ReturnCase{}, false, err
	}
	return commitResult(ctx, tx, p.CompanyID, id, false)
}

func validDisposition(value string) bool {
	return value == "restockable" || value == "damaged" || value == "wrong_product" || value == "missing" || value == "rejected"
}

func lifecycleHash(returnID string, input any) string {
	return requestHash(struct {
		ReturnID string `json:"return_id"`
		Input    any    `json:"input"`
	}{returnID, input})
}

func eventReplay(ctx context.Context, tx pgx.Tx, companyID, returnID, key, hash string) (bool, error) {
	var existingReturnID, existingHash string
	err := tx.QueryRow(ctx, `SELECT return_case_id,request_hash FROM return_events WHERE company_id=$1 AND idempotency_key=$2`, companyID, key).Scan(&existingReturnID, &existingHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if existingReturnID != returnID || existingHash != hash {
		return false, ErrConflict
	}
	return true, nil
}

func commitReplay(ctx context.Context, tx pgx.Tx, companyID, returnID string) (ReturnCase, bool, error) {
	return commitResult(ctx, tx, companyID, returnID, true)
}

func commitResult(ctx context.Context, tx pgx.Tx, companyID, returnID string, replay bool) (ReturnCase, bool, error) {
	item, err := loadReturn(ctx, tx, companyID, returnID)
	if err != nil {
		return ReturnCase{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ReturnCase{}, false, err
	}
	return item, replay, nil
}

func mapInventoryError(err error) error {
	switch {
	case errors.Is(err, authorization.ErrPermissionDenied), errors.Is(err, authorization.ErrModuleUnavailable):
		return err
	case errors.Is(err, inventory.ErrInsufficientStock):
		return ErrInventoryState
	case errors.Is(err, inventory.ErrConflict):
		return ErrConflict
	case errors.Is(err, inventory.ErrInvalidInput), errors.Is(err, inventory.ErrNotFound):
		return ErrInvalidInput
	default:
		return err
	}
}
