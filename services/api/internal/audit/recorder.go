// This file records immutable audit events so important operations remain traceable in the audit package.
package audit

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

type Recorder struct{}

func (Recorder) Record(ctx context.Context, tx pgx.Tx, companyID, actorUserID, action, targetType, targetID string, metadata map[string]any) error {
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_logs (company_id, actor_user_id, action, target_type, target_id, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)`, companyID, actorUserID, action, targetType, targetID, encoded,
	)
	return err
}
