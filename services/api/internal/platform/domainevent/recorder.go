// Package domainevent persists approved transition facts in the caller's
// transaction. It neither evaluates automation rules nor creates print jobs.
package domainevent

import (
	"context"
	"github.com/jackc/pgx/v5"
)

func Record(ctx context.Context, tx pgx.Tx, company, actor, kind, source string, version int) error {
	_, err := tx.Exec(ctx, `INSERT INTO automation_domain_events(company_id,actor_user_id,event_type,source_id,source_version) VALUES($1,$2,$3,$4,$5) ON CONFLICT(company_id,event_type,source_id,source_version) DO NOTHING`, company, actor, kind, source, version)
	return err
}
