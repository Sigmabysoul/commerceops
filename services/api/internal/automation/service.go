// Package automation creates ordinary Printing jobs from approved schedules and
// durable domain facts. It has no Inventory or hardware dependency.
package automation

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/audit"
	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/printing"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidInput = errors.New("invalid automation input")
	ErrNotFound     = errors.New("automation resource not found")
	ErrConflict     = errors.New("automation version or state conflict")
	uuidRE          = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89aAbB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
)

type RuleInput struct {
	Name             string   `json:"name"`
	Enabled          bool     `json:"enabled"`
	Paused           bool     `json:"paused"`
	TriggerType      string   `json:"trigger_type"`
	Schedule         Schedule `json:"schedule"`
	AssetID          string   `json:"asset_id"`
	PrinterID        string   `json:"printer_id"`
	Copies           int      `json:"copies"`
	DailyLimit       *int     `json:"daily_limit"`
	FailureThreshold int      `json:"failure_threshold"`
	BackoffSeconds   int      `json:"backoff_seconds"`
	Version          int      `json:"version"`
}
type Rule struct {
	RuleInput
	ID                  string     `json:"id"`
	Timezone            string     `json:"timezone"`
	CreatedBy           string     `json:"created_by"`
	AssetName           string     `json:"asset_name"`
	PrinterName         string     `json:"printer_name"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	BackoffUntil        *time.Time `json:"backoff_until"`
	NextRunAt           *time.Time `json:"next_run_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}
type Execution struct {
	ID            string     `json:"id"`
	RuleID        string     `json:"rule_id"`
	RuleVersion   int        `json:"rule_version"`
	OccurrenceKey string     `json:"occurrence_key"`
	EventID       *string    `json:"event_id"`
	ScheduledAt   *time.Time `json:"scheduled_at"`
	TestRun       bool       `json:"test_run"`
	Snapshot      Rule       `json:"snapshot"`
	Status        string     `json:"status"`
	AttemptCount  int        `json:"attempt_count"`
	AvailableAt   time.Time  `json:"available_at"`
	Error         *string    `json:"error"`
	PrinterJobID  *string    `json:"printer_job_id"`
	JobStatus     *string    `json:"job_status"`
	CreatedAt     time.Time  `json:"created_at"`
}
type PrintQueue interface {
	QueueAutomationTx(context.Context, pgx.Tx, auth.Principal, string, string, string, int) (printing.Job, error)
}
type Service struct {
	db       *pgxpool.Pool
	authz    *authorization.Service
	printing PrintQueue
	audit    audit.Recorder
}

func NewService(db *pgxpool.Pool, authz *authorization.Service, queue PrintQueue) *Service {
	return &Service{db: db, authz: authz, printing: queue}
}
func (s *Service) require(ctx context.Context, p auth.Principal, manage bool) error {
	permission := "automations.view"
	if manage {
		permission = "automations.manage"
	}
	return s.authz.RequirePermission(ctx, p, permission)
}
func mapError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var p *pgconn.PgError
	if errors.As(err, &p) && (p.Code == "23503" || p.Code == "23514" || p.Code == "22P02") {
		return ErrInvalidInput
	}
	return err
}

const ruleSelect = `SELECT r.id,r.name,r.enabled,r.paused,r.trigger_type,r.schedule,r.timezone,r.asset_id,r.printer_id,r.copies,r.daily_limit,r.failure_threshold,r.backoff_seconds,r.version,r.created_by,r.consecutive_failures,r.backoff_until,r.next_run_at,r.created_at,r.updated_at,a.name,p.friendly_name FROM automation_rules r JOIN print_library_assets a ON a.company_id=r.company_id AND a.id=r.asset_id JOIN registered_printers p ON p.company_id=r.company_id AND p.id=r.printer_id`

func scanRule(row interface{ Scan(...any) error }) (Rule, error) {
	var r Rule
	var raw []byte
	err := row.Scan(&r.ID, &r.Name, &r.Enabled, &r.Paused, &r.TriggerType, &raw, &r.Timezone, &r.AssetID, &r.PrinterID, &r.Copies, &r.DailyLimit, &r.FailureThreshold, &r.BackoffSeconds, &r.Version, &r.CreatedBy, &r.ConsecutiveFailures, &r.BackoffUntil, &r.NextRunAt, &r.CreatedAt, &r.UpdatedAt, &r.AssetName, &r.PrinterName)
	if err != nil {
		return r, mapError(err)
	}
	err = json.Unmarshal(raw, &r.Schedule)
	return r, err
}
func (s *Service) Rules(ctx context.Context, p auth.Principal) ([]Rule, error) {
	if err := s.require(ctx, p, false); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, ruleSelect+` WHERE r.company_id=$1 ORDER BY r.name,r.id`, p.CompanyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Rule{}
	for rows.Next() {
		r, e := scanRule(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Service) Rule(ctx context.Context, p auth.Principal, id string) (Rule, error) {
	if err := s.require(ctx, p, false); err != nil {
		return Rule{}, err
	}
	if !uuidRE.MatchString(id) {
		return Rule{}, ErrInvalidInput
	}
	return scanRule(s.db.QueryRow(ctx, ruleSelect+` WHERE r.company_id=$1 AND r.id=$2`, p.CompanyID, id))
}
func validInput(in RuleInput) bool {
	if in.Name == "" || len(in.Name) > 120 || !uuidRE.MatchString(in.AssetID) || !uuidRE.MatchString(in.PrinterID) || in.Copies < 1 || in.Copies > printing.MaxCopies || in.FailureThreshold < 1 || in.FailureThreshold > 20 || in.BackoffSeconds < 1 || in.BackoffSeconds > 3600 {
		return false
	}
	if in.DailyLimit != nil && (*in.DailyLimit < in.Copies || *in.DailyLimit > 10000) {
		return false
	}
	switch in.TriggerType {
	case "scheduled":
		return in.Schedule.validate() == nil
	case "ecommerce_batch_ready", "consignment_packing", "consignment_packed":
		return in.Schedule.Mode == "" && len(in.Schedule.Times) == 0 && len(in.Schedule.Weekdays) == 0 && in.Schedule.StartDate == "" && in.Schedule.EndDate == ""
	}
	return false
}
func (s *Service) SaveRule(ctx context.Context, p auth.Principal, id string, in RuleInput) (Rule, error) {
	if err := s.require(ctx, p, true); err != nil {
		return Rule{}, err
	}
	in.Name = strings.TrimSpace(in.Name)
	if !validInput(in) || id != "" && (!uuidRE.MatchString(id) || in.Version < 1) || id == "" && in.Version != 0 {
		return Rule{}, ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Rule{}, err
	}
	defer tx.Rollback(ctx)
	var zone string
	// Serialize creation/timezone edits without blocking FK key-share locks
	// held by execution inserts. A stronger company lock would invert the
	// company/rule lock order against the worker. Rule count bounds event fan-out.
	if err = tx.QueryRow(ctx, `SELECT timezone FROM companies WHERE id=$1 FOR NO KEY UPDATE`, p.CompanyID).Scan(&zone); err != nil {
		return Rule{}, mapError(err)
	}
	if _, err = location(zone); err != nil {
		return Rule{}, err
	}
	var previous *Rule
	if id != "" {
		old, e := scanRule(tx.QueryRow(ctx, ruleSelect+` WHERE r.company_id=$1 AND r.id=$2 FOR UPDATE OF r`, p.CompanyID, id))
		if e != nil {
			return Rule{}, e
		}
		if old.Version != in.Version {
			return Rule{}, ErrConflict
		}
		previous = &old
	} else {
		var count int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM automation_rules WHERE company_id=$1`, p.CompanyID).Scan(&count); err != nil {
			return Rule{}, err
		}
		if count >= 100 {
			return Rule{}, ErrConflict
		}
	}
	var valid bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM print_library_assets a JOIN registered_printers p ON p.company_id=a.company_id WHERE a.company_id=$1 AND a.id=$2 AND a.active AND p.id=$3)`, p.CompanyID, in.AssetID, in.PrinterID).Scan(&valid); err != nil {
		return Rule{}, err
	}
	if !valid {
		return Rule{}, ErrInvalidInput
	}
	var next *time.Time
	if in.TriggerType == "scheduled" && in.Enabled && !in.Paused {
		next, err = NextRun(in.Schedule, zone, time.Now())
		if err != nil {
			return Rule{}, err
		}
	}
	raw, err := json.Marshal(in.Schedule)
	if err != nil {
		return Rule{}, err
	}
	if id == "" {
		err = tx.QueryRow(ctx, `INSERT INTO automation_rules(company_id,name,enabled,paused,trigger_type,schedule,timezone,asset_id,printer_id,copies,daily_limit,failure_threshold,backoff_seconds,created_by,next_run_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING id`, p.CompanyID, in.Name, in.Enabled, in.Paused, in.TriggerType, raw, zone, in.AssetID, in.PrinterID, in.Copies, in.DailyLimit, in.FailureThreshold, in.BackoffSeconds, p.UserID, next).Scan(&id)
	} else {
		_, err = tx.Exec(ctx, `UPDATE automation_rules SET name=$3,enabled=$4,paused=$5,trigger_type=$6,schedule=$7,timezone=$8,asset_id=$9,printer_id=$10,copies=$11,daily_limit=$12,failure_threshold=$13,backoff_seconds=$14,next_run_at=$15,version=version+1,consecutive_failures=0,backoff_until=NULL,activated_at=clock_timestamp(),updated_at=clock_timestamp() WHERE company_id=$1 AND id=$2`, p.CompanyID, id, in.Name, in.Enabled, in.Paused, in.TriggerType, raw, zone, in.AssetID, in.PrinterID, in.Copies, in.DailyLimit, in.FailureThreshold, in.BackoffSeconds, next)
	}
	if err != nil {
		return Rule{}, mapError(err)
	}
	result, err := scanRule(tx.QueryRow(ctx, ruleSelect+` WHERE r.company_id=$1 AND r.id=$2`, p.CompanyID, id))
	if err != nil {
		return Rule{}, err
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "automation.rule_saved", "automation_rule", id, map[string]any{"previous": previous, "rule": result}); err != nil {
		return Rule{}, err
	}
	return result, tx.Commit(ctx)
}
func (s *Service) Timezone(ctx context.Context, p auth.Principal) (string, error) {
	if err := s.require(ctx, p, false); err != nil {
		return "", err
	}
	var zone string
	err := s.db.QueryRow(ctx, `SELECT timezone FROM companies WHERE id=$1`, p.CompanyID).Scan(&zone)
	return zone, mapError(err)
}
func (s *Service) SetTimezone(ctx context.Context, p auth.Principal, zone string) error {
	if err := s.require(ctx, p, true); err != nil {
		return err
	}
	if _, err := location(zone); err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var old string
	if err = tx.QueryRow(ctx, `SELECT timezone FROM companies WHERE id=$1 FOR NO KEY UPDATE`, p.CompanyID).Scan(&old); err != nil {
		return mapError(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE companies SET timezone=$2 WHERE id=$1`, p.CompanyID, zone); err != nil {
		return err
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "automation.timezone_changed", "company", p.CompanyID, map[string]any{"previous": old, "timezone": zone}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) Preview(ctx context.Context, p auth.Principal, schedule Schedule) ([]time.Time, error) {
	zone, err := s.Timezone(ctx, p)
	if err != nil {
		return nil, err
	}
	out := []time.Time{}
	after := time.Now()
	for n := 0; n < 10; n++ {
		next, e := NextRun(schedule, zone, after)
		if e != nil {
			return nil, e
		}
		if next == nil {
			break
		}
		out = append(out, *next)
		after = *next
	}
	return out, nil
}
