package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/printing"
	"github.com/jackc/pgx/v5"
)

// Run is one bounded worker per API process. Cancellation ends polling and
// database work; the application waits for it before closing the pool.
func (s *Service) Run(ctx context.Context, logger *slog.Logger) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := s.Tick(ctx); err != nil && ctx.Err() == nil {
			logger.Error("automation scheduler tick failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Service) Tick(ctx context.Context) error {
	for n := 0; n < 32; n++ {
		ok, err := s.materializeSchedule(ctx)
		if err != nil {
			return err
		}
		if !ok {
			break
		}
	}
	for n := 0; n < 32; n++ {
		ok, err := s.materializeEvent(ctx)
		if err != nil {
			return err
		}
		if !ok {
			break
		}
	}
	for n := 0; n < 32; n++ {
		c, err := s.claim(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			break
		}
		if err != nil {
			return err
		}
		if err = s.execute(ctx, c); err != nil {
			return err
		}
	}
	return nil
}
func insertExecution(ctx context.Context, tx pgx.Tx, company string, r Rule, key string, event *string, scheduled *time.Time, test bool) (string, error) {
	raw, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO automation_executions(company_id,rule_id,rule_version,occurrence_key,event_id,scheduled_at,test_run,snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(company_id,rule_id,occurrence_key) DO NOTHING RETURNING id`, company, r.ID, r.Version, key, event, scheduled, test, raw).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id FROM automation_executions WHERE company_id=$1 AND rule_id=$2 AND occurrence_key=$3`, company, r.ID, key).Scan(&id)
	}
	return id, err
}
func (s *Service) materializeSchedule(ctx context.Context) (bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var company, id string
	err = tx.QueryRow(ctx, `SELECT company_id,id FROM automation_rules WHERE enabled AND NOT paused AND trigger_type='scheduled' AND next_run_at<=now() ORDER BY next_run_at,id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&company, &id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	r, err := scanRule(tx.QueryRow(ctx, ruleSelect+` WHERE r.company_id=$1 AND r.id=$2`, company, id))
	if err != nil {
		return false, err
	}
	loc, err := location(r.Timezone)
	if err != nil {
		return false, err
	}
	key := "schedule:" + r.NextRunAt.In(loc).Format("2006-01-02:15:04")
	if _, err = insertExecution(ctx, tx, company, r, key, nil, r.NextRunAt, false); err != nil {
		return false, err
	}
	next, err := NextRun(r.Schedule, r.Timezone, *r.NextRunAt)
	if err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE automation_rules SET next_run_at=$3 WHERE company_id=$1 AND id=$2`, company, id, next); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
func (s *Service) materializeEvent(ctx context.Context) (bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var company, id, kind string
	var occurred time.Time
	err = tx.QueryRow(ctx, `SELECT company_id,id,event_type,occurred_at FROM automation_domain_events WHERE processed_at IS NULL ORDER BY occurred_at,id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&company, &id, &kind, &occurred)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// Only rules active before the fact occurred match. Enabling/editing a rule
	// never prints historical events using a newly configured asset.
	rows, err := tx.Query(ctx, ruleSelect+` WHERE r.company_id=$1 AND r.trigger_type=$2 AND r.enabled AND NOT r.paused AND r.activated_at<=$3 ORDER BY r.id FOR UPDATE OF r`, company, kind, occurred)
	if err != nil {
		return false, err
	}
	rules := []Rule{}
	for rows.Next() {
		r, e := scanRule(rows)
		if e != nil {
			rows.Close()
			return false, e
		}
		rules = append(rules, r)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return false, err
	}
	for _, r := range rules {
		if _, err = insertExecution(ctx, tx, company, r, "event:"+id, &id, nil, false); err != nil {
			return false, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE automation_domain_events SET processed_at=now() WHERE company_id=$1 AND id=$2`, company, id); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

type claim struct{ company, id, rule, token string }

func (s *Service) claim(ctx context.Context) (claim, error) {
	var c claim
	err := s.db.QueryRow(ctx, `WITH candidate AS (
 SELECT e.id FROM automation_executions e JOIN automation_rules r ON r.company_id=e.company_id AND r.id=e.rule_id
 WHERE ((e.status IN ('pending','failed') AND e.available_at<=now()) OR (e.status='running' AND e.lease_expires_at<=now()))
 AND (e.status<>'failed' OR (r.enabled AND NOT r.paused)) AND (r.backoff_until IS NULL OR r.backoff_until<=now())
 ORDER BY e.available_at,e.created_at,e.id FOR UPDATE OF e SKIP LOCKED LIMIT 1)
 UPDATE automation_executions e SET status='running',lease_token=gen_random_uuid(),lease_expires_at=now()+interval '1 minute',attempt_count=attempt_count+1,updated_at=now() FROM candidate c WHERE e.id=c.id RETURNING e.company_id,e.id,e.rule_id,e.lease_token`).Scan(&c.company, &c.id, &c.rule, &c.token)
	return c, err
}
func (s *Service) execute(ctx context.Context, c claim) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	r, err := scanRule(tx.QueryRow(ctx, ruleSelect+` WHERE r.company_id=$1 AND r.id=$2 FOR UPDATE OF r`, c.company, c.rule))
	if err != nil {
		return err
	}
	var raw []byte
	var test bool
	err = tx.QueryRow(ctx, `SELECT snapshot,test_run FROM automation_executions WHERE company_id=$1 AND id=$2 AND status='running' AND lease_token=$3 AND lease_expires_at>now() FOR UPDATE`, c.company, c.id, c.token).Scan(&raw, &test)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var snapshot Rule
	if err = json.Unmarshal(raw, &snapshot); err != nil {
		return err
	}
	actor := auth.Principal{CompanyID: c.company, UserID: snapshot.CreatedBy}
	status, reason := "completed", ""
	var jobID *string
	if !test && (!r.Enabled || r.Paused) {
		status, reason = "skipped", "Rule is disabled or paused"
	}
	if status == "completed" && r.BackoffUntil != nil && r.BackoffUntil.After(time.Now()) {
		_, err = tx.Exec(ctx, `UPDATE automation_executions SET status='pending',available_at=$3,lease_token=NULL,lease_expires_at=NULL WHERE company_id=$1 AND id=$2`, c.company, c.id, r.BackoffUntil)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if status == "completed" && r.DailyLimit != nil {
		// Holding the rule lock serializes the derived daily budget across workers.
		var copies int
		err = tx.QueryRow(ctx, `SELECT COALESCE(sum(j.copies),0) FROM printer_jobs j JOIN automation_executions e ON e.company_id=j.company_id AND e.id::text=j.origin_reference WHERE e.company_id=$1 AND e.rule_id=$2 AND j.origin_type='automation' AND (j.created_at AT TIME ZONE $3)::date=(now() AT TIME ZONE $3)::date`, c.company, c.rule, r.Timezone).Scan(&copies)
		if err != nil {
			return err
		}
		if copies+snapshot.Copies > *r.DailyLimit {
			status, reason = "skipped", "Daily copy limit reached"
		}
	}
	if status == "completed" {
		// Savepoint permits recording a domain failure without losing the lease or
		// leaving any partial Printing insertion/audit in this transaction.
		child, e := tx.Begin(ctx)
		if e != nil {
			return e
		}
		job, e := s.printing.QueueAutomationTx(ctx, child, actor, c.id, snapshot.PrinterID, snapshot.AssetID, snapshot.Copies)
		if e != nil {
			if rollbackErr := child.Rollback(ctx); rollbackErr != nil {
				return rollbackErr
			}
			if !errors.Is(e, printing.ErrNotFound) && !errors.Is(e, printing.ErrInvalidInput) && !errors.Is(e, printing.ErrConflict) {
				return fmt.Errorf("queue automation execution %s: %w", c.id, e)
			}
			status, reason = "failed", "Printer is offline/disabled or the configured asset is unavailable"
		} else {
			if err = child.Commit(ctx); err != nil {
				return err
			}
			jobID = &job.ID
		}
	}
	var available time.Time
	if status == "failed" {
		failures := r.ConsecutiveFailures + 1
		seconds := r.BackoffSeconds
		for i := 1; i < failures && seconds < 3600; i++ {
			seconds *= 2
		}
		if seconds > 3600 {
			seconds = 3600
		}
		available = time.Now().Add(time.Duration(seconds) * time.Second)
		pause := failures >= r.FailureThreshold
		if _, err = tx.Exec(ctx, `UPDATE automation_rules SET consecutive_failures=$3,backoff_until=$4,paused=paused OR $5,version=version+CASE WHEN $5 THEN 1 ELSE 0 END,updated_at=now() WHERE company_id=$1 AND id=$2`, c.company, c.rule, failures, available, pause); err != nil {
			return err
		}
		if pause {
			if err = s.audit.Record(ctx, tx, c.company, actor.UserID, "automation.auto_paused", "automation_rule", c.rule, map[string]any{"failures": failures, "execution_id": c.id}); err != nil {
				return err
			}
		}
	} else {
		available = time.Now()
		if status == "completed" {
			if _, err = tx.Exec(ctx, `UPDATE automation_rules SET consecutive_failures=0,backoff_until=NULL WHERE company_id=$1 AND id=$2`, c.company, c.rule); err != nil {
				return err
			}
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE automation_executions SET status=$3,error=NULLIF($4,''),printer_job_id=$5,available_at=$6,lease_token=NULL,lease_expires_at=NULL,updated_at=now() WHERE company_id=$1 AND id=$2`, c.company, c.id, status, reason, jobID, available); err != nil {
		return err
	}
	if err = s.audit.Record(ctx, tx, c.company, actor.UserID, "automation.execution_"+status, "automation_execution", c.id, map[string]any{"rule_id": c.rule, "rule_version": snapshot.Version, "printer_job_id": jobID, "reason": reason}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) TestRun(ctx context.Context, p auth.Principal, id, key string) (string, error) {
	if err := s.require(ctx, p, true); err != nil {
		return "", err
	}
	if !uuidRE.MatchString(id) || key == "" || stringsInvalidKey(key) {
		return "", ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	r, err := scanRule(tx.QueryRow(ctx, ruleSelect+` WHERE r.company_id=$1 AND r.id=$2 FOR UPDATE OF r`, p.CompanyID, id))
	if err != nil {
		return "", err
	}
	// A test is explicitly authorized by its requester, including on paused rules.
	r.CreatedBy = p.UserID
	execution, err := insertExecution(ctx, tx, p.CompanyID, r, "test:"+key, nil, nil, true)
	if err != nil {
		return "", err
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "automation.test_requested", "automation_execution", execution, map[string]any{"rule_id": id}); err != nil {
		return "", err
	}
	return execution, tx.Commit(ctx)
}
func stringsInvalidKey(key string) bool { return len(key) > 128 || key != strings.TrimSpace(key) }
