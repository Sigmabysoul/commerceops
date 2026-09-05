package automation

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/auth"
)

func (s *Service) Runs(ctx context.Context, p auth.Principal, ruleID string, failures bool) ([]Execution, error) {
	if err := s.require(ctx, p, false); err != nil {
		return nil, err
	}
	if ruleID != "" && !uuidRE.MatchString(ruleID) {
		return nil, ErrInvalidInput
	}
	rows, err := s.db.Query(ctx, `SELECT e.id,e.rule_id,e.rule_version,e.occurrence_key,e.event_id,e.scheduled_at,e.test_run,e.snapshot,e.status,e.attempt_count,e.available_at,COALESCE(e.error,j.failure_message),e.printer_job_id,j.status,e.created_at FROM automation_executions e LEFT JOIN printer_jobs j ON j.company_id=e.company_id AND j.id=e.printer_job_id WHERE e.company_id=$1 AND ($2='' OR e.rule_id::text=$2) AND (NOT $3 OR e.status='failed' OR j.status='failed') ORDER BY e.created_at DESC,e.id DESC LIMIT 200`, p.CompanyID, ruleID, failures)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Execution{}
	for rows.Next() {
		var e Execution
		var raw []byte
		if err = rows.Scan(&e.ID, &e.RuleID, &e.RuleVersion, &e.OccurrenceKey, &e.EventID, &e.ScheduledAt, &e.TestRun, &raw, &e.Status, &e.AttemptCount, &e.AvailableAt, &e.Error, &e.PrinterJobID, &e.JobStatus, &e.CreatedAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &e.Snapshot); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
func (s *Service) Retry(ctx context.Context, p auth.Principal, id string) error {
	if err := s.require(ctx, p, true); err != nil {
		return err
	}
	if !uuidRE.MatchString(id) {
		return ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var rule string
	if err = tx.QueryRow(ctx, `SELECT rule_id FROM automation_executions WHERE company_id=$1 AND id=$2`, p.CompanyID, id).Scan(&rule); err != nil {
		return mapError(err)
	}
	r, err := scanRule(tx.QueryRow(ctx, ruleSelect+` WHERE r.company_id=$1 AND r.id=$2 FOR UPDATE OF r`, p.CompanyID, rule))
	if err != nil {
		return err
	}
	var status string
	if err = tx.QueryRow(ctx, `SELECT status FROM automation_executions WHERE company_id=$1 AND id=$2 FOR UPDATE`, p.CompanyID, id).Scan(&status); err != nil {
		return mapError(err)
	}
	// Completed here means a normal job exists, regardless of its physical state.
	// Physical failure requires the Printing domain's explicit reprint workflow.
	if status == "completed" {
		return nil
	}
	if status != "failed" || !r.Enabled || r.Paused {
		return ErrConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE automation_executions SET status='pending',available_at=GREATEST(now(),$3::timestamptz),updated_at=now() WHERE company_id=$1 AND id=$2`, p.CompanyID, id, r.BackoffUntil); err != nil {
		return err
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "automation.retry_requested", "automation_execution", id, map[string]any{"rule_id": rule}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type History struct {
	ID       string          `json:"id"`
	Action   string          `json:"action"`
	Actor    string          `json:"actor_user_id"`
	At       time.Time       `json:"occurred_at"`
	Metadata json.RawMessage `json:"metadata"`
}

func (s *Service) History(ctx context.Context, p auth.Principal, id string) ([]History, error) {
	if _, err := s.Rule(ctx, p, id); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT id,action,actor_user_id,occurred_at,metadata FROM audit_logs WHERE company_id=$1 AND target_type='automation_rule' AND target_id=$2 ORDER BY occurred_at DESC,id DESC LIMIT 200`, p.CompanyID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []History{}
	for rows.Next() {
		var h History
		if err = rows.Scan(&h.ID, &h.Action, &h.Actor, &h.At, &h.Metadata); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

type Metric struct {
	Origin        string `json:"origin"`
	PrinterID     string `json:"printer_id"`
	PrinterName   string `json:"printer_name"`
	Jobs          int64  `json:"jobs"`
	Copies        int64  `json:"copies"`
	Completed     int64  `json:"completed"`
	Failed        int64  `json:"failed"`
	Cancelled     int64  `json:"cancelled"`
	Pending       int64  `json:"pending"`
	FailureEvents int64  `json:"failure_events"`
}

func (s *Service) Report(ctx context.Context, p auth.Principal) ([]Metric, error) {
	if err := s.require(ctx, p, false); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT CASE WHEN j.origin_type='automation' THEN 'automatic' ELSE 'manual' END,p.id,p.friendly_name,count(*),sum(j.copies),count(*) FILTER(WHERE j.status='completed'),count(*) FILTER(WHERE j.status='failed'),count(*) FILTER(WHERE j.status='cancelled'),count(*) FILTER(WHERE j.status IN ('queued','claimed','printing')),COALESCE(sum(ev.failures),0) FROM printer_jobs j JOIN registered_printers p ON p.company_id=j.company_id AND p.id=j.printer_id LEFT JOIN LATERAL (SELECT count(*) AS failures FROM printer_job_events e WHERE e.company_id=j.company_id AND e.printer_job_id=j.id AND e.event_type IN ('failed','lease_expired')) ev ON true WHERE j.company_id=$1 GROUP BY 1,p.id,p.friendly_name ORDER BY p.friendly_name,1`, p.CompanyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Metric{}
	for rows.Next() {
		var m Metric
		if err = rows.Scan(&m.Origin, &m.PrinterID, &m.PrinterName, &m.Jobs, &m.Copies, &m.Completed, &m.Failed, &m.Cancelled, &m.Pending, &m.FailureEvents); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

type Upcoming struct {
	Rule Rule      `json:"rule"`
	At   time.Time `json:"at"`
}

func (s *Service) Upcoming(ctx context.Context, p auth.Principal) ([]Upcoming, error) {
	rules, err := s.Rules(ctx, p)
	if err != nil {
		return nil, err
	}
	out := []Upcoming{}
	for _, r := range rules {
		if r.Enabled && !r.Paused && r.NextRunAt != nil {
			out = append(out, Upcoming{Rule: r, At: *r.NextRunAt})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

// Pause preserves the configured timezone and asset strategy. Resuming resets
// backoff and schedules future work; it does not silently adopt company edits.
func (s *Service) Pause(ctx context.Context, p auth.Principal, id string, version int, paused bool) (Rule, error) {
	if err := s.require(ctx, p, true); err != nil {
		return Rule{}, err
	}
	if !uuidRE.MatchString(id) || version < 1 {
		return Rule{}, ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Rule{}, err
	}
	defer tx.Rollback(ctx)
	old, err := scanRule(tx.QueryRow(ctx, ruleSelect+` WHERE r.company_id=$1 AND r.id=$2 FOR UPDATE OF r`, p.CompanyID, id))
	if err != nil {
		return Rule{}, err
	}
	if old.Version != version {
		return Rule{}, ErrConflict
	}
	var next *time.Time
	if !paused && old.Enabled && old.TriggerType == "scheduled" {
		next, err = NextRun(old.Schedule, old.Timezone, time.Now())
		if err != nil {
			return Rule{}, err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE automation_rules SET paused=$3,version=version+1,next_run_at=$4,consecutive_failures=CASE WHEN $3 THEN consecutive_failures ELSE 0 END,backoff_until=CASE WHEN $3 THEN backoff_until ELSE NULL END,activated_at=clock_timestamp(),updated_at=clock_timestamp() WHERE company_id=$1 AND id=$2`, p.CompanyID, id, paused, next)
	if err != nil {
		return Rule{}, err
	}
	result, err := scanRule(tx.QueryRow(ctx, ruleSelect+` WHERE r.company_id=$1 AND r.id=$2`, p.CompanyID, id))
	if err != nil {
		return Rule{}, err
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "automation.pause_changed", "automation_rule", id, map[string]any{"previous": old, "rule": result}); err != nil {
		return Rule{}, err
	}
	return result, tx.Commit(ctx)
}

type AssetOption struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}
type PrinterOption struct {
	ID           string `json:"id"`
	FriendlyName string `json:"friendly_name"`
	Status       string `json:"status"`
	Enabled      bool   `json:"enabled"`
}
type Options struct {
	Assets   []AssetOption   `json:"assets"`
	Printers []PrinterOption `json:"printers"`
}

// Options exposes only the tenant-owned friendly selections needed to configure
// a rule. Managing automations does not require granting printer administration.
func (s *Service) Options(ctx context.Context, p auth.Principal) (Options, error) {
	out := Options{Assets: []AssetOption{}, Printers: []PrinterOption{}}
	if err := s.require(ctx, p, true); err != nil {
		return out, err
	}
	rows, err := s.db.Query(ctx, `SELECT id,name,active FROM print_library_assets WHERE company_id=$1 ORDER BY name,id`, p.CompanyID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var a AssetOption
		if err = rows.Scan(&a.ID, &a.Name, &a.Active); err != nil {
			rows.Close()
			return out, err
		}
		out.Assets = append(out.Assets, a)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return out, err
	}
	rows, err = s.db.Query(ctx, `SELECT id,friendly_name,CASE WHEN last_seen_at>now()-interval '90 seconds' THEN status ELSE 'offline' END,enabled FROM registered_printers WHERE company_id=$1 ORDER BY friendly_name,id`, p.CompanyID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v PrinterOption
		if err = rows.Scan(&v.ID, &v.FriendlyName, &v.Status, &v.Enabled); err != nil {
			return out, err
		}
		out.Printers = append(out.Printers, v)
	}
	return out, rows.Err()
}
