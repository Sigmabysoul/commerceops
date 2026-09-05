// This file calculates return closure outcomes while preserving inventory and audit invariants in the returns package.
package returns

import (
	"context"
	"errors"
	"strings"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/jackc/pgx/v5"
)

type CloseInput struct {
	Notes          *string `json:"notes"`
	IdempotencyKey string  `json:"idempotency_key"`
}

func (s *Service) CloseReturn(ctx context.Context, p auth.Principal, id string, input CloseInput) (ReturnCase, bool, error) {
	if err := s.authorize(ctx, p, "returns.manage"); err != nil {
		return ReturnCase{}, false, err
	}
	id = strings.TrimSpace(id)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.Notes = trimOptional(input.Notes)
	if !validCloseInput(id, input) {
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
	if status != "restocked" && status != "restock_corrected" && status != "damaged" && status != "rejected" {
		return ReturnCase{}, false, ErrInvalidState
	}
	if _, err = tx.Exec(ctx, `INSERT INTO return_events(company_id,return_case_id,event_type,actor_user_id,notes,idempotency_key,request_hash) VALUES($1,$2,'closed',$3,$4,$5,$6)`, p.CompanyID, id, p.UserID, input.Notes, input.IdempotencyKey, hash); err != nil {
		return ReturnCase{}, false, mapDBError(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE return_cases SET status='closed',closed_by=$1,closed_at=now(),updated_at=now() WHERE company_id=$2 AND id=$3`, p.UserID, p.CompanyID, id); err != nil {
		return ReturnCase{}, false, err
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "returns.closed", "return_case", id, map[string]any{"previous_status": status}); err != nil {
		return ReturnCase{}, false, err
	}
	return commitResult(ctx, tx, p.CompanyID, id, false)
}

func (s *Service) CloseCancellation(ctx context.Context, p auth.Principal, id string, input CloseInput) (Cancellation, bool, error) {
	if err := s.authorize(ctx, p, "returns.manage"); err != nil {
		return Cancellation{}, false, err
	}
	id = strings.TrimSpace(id)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.Notes = trimOptional(input.Notes)
	if !validCloseInput(id, input) {
		return Cancellation{}, false, ErrInvalidInput
	}
	hash := requestHash(struct {
		CancellationID string     `json:"cancellation_id"`
		Input          CloseInput `json:"input"`
	}{id, input})
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Cancellation{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if replay, replayErr := cancellationEventReplay(ctx, tx, p.CompanyID, id, input.IdempotencyKey, hash); replayErr != nil {
		return Cancellation{}, false, replayErr
	} else if replay {
		return commitCancellationResult(ctx, tx, p.CompanyID, id, true)
	}
	var status string
	if err = tx.QueryRow(ctx, `SELECT status FROM cancellations WHERE company_id=$1 AND id=$2 FOR UPDATE`, p.CompanyID, id).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return Cancellation{}, false, ErrNotFound
	} else if err != nil {
		return Cancellation{}, false, err
	}
	if replay, replayErr := cancellationEventReplay(ctx, tx, p.CompanyID, id, input.IdempotencyKey, hash); replayErr != nil {
		return Cancellation{}, false, replayErr
	} else if replay {
		return commitCancellationResult(ctx, tx, p.CompanyID, id, true)
	}
	if status != "recorded" {
		return Cancellation{}, false, ErrInvalidState
	}
	if _, err = tx.Exec(ctx, `INSERT INTO cancellation_events(company_id,cancellation_id,event_type,actor_user_id,notes,idempotency_key,request_hash) VALUES($1,$2,'closed',$3,$4,$5,$6)`, p.CompanyID, id, p.UserID, input.Notes, input.IdempotencyKey, hash); err != nil {
		return Cancellation{}, false, mapDBError(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE cancellations SET status='closed',closed_by=$1,closed_at=now(),updated_at=now() WHERE company_id=$2 AND id=$3`, p.UserID, p.CompanyID, id); err != nil {
		return Cancellation{}, false, err
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "returns.cancellation_closed", "cancellation", id, map[string]any{}); err != nil {
		return Cancellation{}, false, err
	}
	return commitCancellationResult(ctx, tx, p.CompanyID, id, false)
}

func validCloseInput(id string, input CloseInput) bool {
	return uuidRE.MatchString(id) && input.IdempotencyKey != "" && len(input.IdempotencyKey) <= 128 && (input.Notes == nil || len(*input.Notes) <= 2000)
}

func cancellationEventReplay(ctx context.Context, tx pgx.Tx, companyID, cancellationID, key, hash string) (bool, error) {
	var existingCancellationID, existingHash string
	err := tx.QueryRow(ctx, `SELECT cancellation_id,request_hash FROM cancellation_events WHERE company_id=$1 AND idempotency_key=$2`, companyID, key).Scan(&existingCancellationID, &existingHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if existingCancellationID != cancellationID || existingHash != hash {
		return false, ErrConflict
	}
	return true, nil
}

func commitCancellationResult(ctx context.Context, tx pgx.Tx, companyID, cancellationID string, replay bool) (Cancellation, bool, error) {
	item, err := loadCancellation(ctx, tx, companyID, cancellationID)
	if err != nil {
		return Cancellation{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Cancellation{}, false, err
	}
	return item, replay, nil
}
