package automation

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/platform/domainevent"
	"github.com/commerceops/commerceops/services/api/internal/platform/objectstorage"
	"github.com/commerceops/commerceops/services/api/internal/printing"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	db                   *pgxpool.Pool
	s                    *Service
	print                *printing.Service
	p, other             auth.Principal
	role, asset, printer string
	agent                printing.AgentPrincipal
	pdf                  []byte
}

func check(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func sql(t *testing.T, db *pgxpool.Pool, q string, args ...any) {
	t.Helper()
	_, err := db.Exec(context.Background(), q, args...)
	check(t, err)
}
func scalar[T any](t *testing.T, db *pgxpool.Pool, q string, args ...any) T {
	t.Helper()
	var v T
	check(t, db.QueryRow(context.Background(), q, args...).Scan(&v))
	return v
}
func setup(t *testing.T) *fixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	check(t, err)
	schema := fmt.Sprintf("automation_test_%d", time.Now().UnixNano())
	sql(t, admin, `CREATE SCHEMA `+schema)
	cfg, err := pgxpool.ParseConfig(url)
	check(t, err)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	check(t, err)
	t.Cleanup(func() {
		db.Close()
		_, e := admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`)
		if e != nil {
			t.Error(e)
		}
		admin.Close()
	})
	migrations, err := filepath.Glob("../../migrations/*.up.sql")
	check(t, err)
	for _, file := range migrations {
		data, e := os.ReadFile(file)
		check(t, e)
		sql(t, db, string(data))
	}
	f := &fixture{db: db}
	f.p.CompanyID = scalar[string](t, db, `INSERT INTO companies(name) VALUES('Automation') RETURNING id`)
	f.other.CompanyID = scalar[string](t, db, `INSERT INTO companies(name) VALUES('Other') RETURNING id`)
	f.p.UserID = scalar[string](t, db, `INSERT INTO users(email,password_hash) VALUES('automation@example.test','test') RETURNING id`)
	f.other.UserID = f.p.UserID
	for _, p := range []auth.Principal{f.p, f.other} {
		sql(t, db, `INSERT INTO company_users(company_id,user_id) VALUES($1,$2)`, p.CompanyID, p.UserID)
		role := scalar[string](t, db, `INSERT INTO roles(company_id,name) VALUES($1,'Manager') RETURNING id`, p.CompanyID)
		sql(t, db, `INSERT INTO role_permissions(company_id,role_id,permission_key) SELECT $1,$2,key FROM permissions`, p.CompanyID, role)
		sql(t, db, `INSERT INTO company_user_roles(company_id,user_id,role_id) VALUES($1,$2,$3)`, p.CompanyID, p.UserID, role)
		if p.CompanyID == f.p.CompanyID {
			f.role = role
		}
	}
	storage, err := objectstorage.NewLocal(t.TempDir())
	check(t, err)
	az := authorization.NewService(db)
	f.print = printing.NewService(db, az, storage)
	f.s = NewService(db, az, f.print)
	a, err := f.print.CreateAgent(ctx, f.p, "Packing agent", "linux_cups")
	check(t, err)
	f.agent = printing.AgentPrincipal{CompanyID: f.p.CompanyID, AgentID: a.Agent.ID}
	printers, err := f.print.Heartbeat(ctx, f.agent, []printing.LocalPrinter{{OSPrinterID: "fixture", SuggestedName: "Packing printer", Capabilities: map[string]any{}}})
	check(t, err)
	f.printer = printers[0].ID
	f.pdf, err = os.ReadFile("../marketplace/amazon/testdata/sanitized_label_invoice.pdf")
	check(t, err)
	asset, err := f.print.CreateAsset(ctx, f.p, "Packing sticker", "Packing", "", nil, 1, nil, false, "sticker.pdf", f.pdf)
	check(t, err)
	f.asset = asset.ID
	return f
}
func (f *fixture) rule(t *testing.T, trigger string) Rule {
	t.Helper()
	in := RuleInput{Name: "Rule", Enabled: true, TriggerType: trigger, AssetID: f.asset, PrinterID: f.printer, Copies: 2, FailureThreshold: 3, BackoffSeconds: 1}
	if trigger == "scheduled" {
		in.Schedule = Schedule{Mode: "daily", Times: []string{"09:00"}}
	}
	r, err := f.s.SaveRule(context.Background(), f.p, "", in)
	check(t, err)
	return r
}
func (f *fixture) test(t *testing.T, r Rule, key string) string {
	t.Helper()
	id, err := f.s.TestRun(context.Background(), f.p, r.ID, key)
	check(t, err)
	return id
}
func (f *fixture) count(t *testing.T, table string) int {
	return scalar[int](t, f.db, `SELECT count(*) FROM `+table+` WHERE company_id=$1`, f.p.CompanyID)
}
func TestSchedulerConcurrentRestartAndAgentDelivery(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	r := f.rule(t, "scheduled")
	due := time.Now().UTC().AddDate(0, 0, -1)
	due = time.Date(due.Year(), due.Month(), due.Day(), 9, 0, 0, 0, time.UTC)
	sql(t, f.db, `UPDATE automation_rules SET next_run_at=$2 WHERE id=$1`, r.ID, due)
	ok, err := f.s.materializeSchedule(ctx)
	check(t, err)
	if !ok {
		t.Fatal("not materialized")
	}
	// Restart after materialization, then lose a lease before creating the job.
	c, err := f.s.claim(ctx)
	check(t, err)
	sql(t, f.db, `UPDATE automation_executions SET lease_expires_at=now()-interval '1 second' WHERE id=$1`, c.id)
	restarted := NewService(f.db, authorization.NewService(f.db), f.print)
	replacement, err := restarted.claim(ctx)
	check(t, err)
	check(t, f.s.execute(ctx, c))
	if f.count(t, "printer_jobs") != 0 {
		t.Fatal("stale lease created job")
	}
	check(t, restarted.execute(ctx, replacement))
	check(t, restarted.execute(ctx, replacement))
	if f.count(t, "printer_jobs") != 1 {
		t.Fatal("duplicate queue creation")
	}
	// Concurrent replays of one test occurrence also create exactly one job.
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := f.s.TestRun(ctx, f.p, r.ID, "same-request")
			if e == nil {
				c, e2 := f.s.claim(ctx)
				if e2 == nil {
					e = f.s.execute(ctx, c)
				} else if !errors.Is(e2, pgx.ErrNoRows) {
					e = e2
				}
			}
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		check(t, e)
	}
	if f.count(t, "printer_jobs") != 2 {
		t.Fatal("test replay duplicated job")
	}
	runs, err := f.s.Runs(ctx, f.p, "", false)
	check(t, err)
	for _, run := range runs {
		check(t, f.s.Retry(ctx, f.p, run.ID))
	}
	if f.count(t, "printer_jobs") != 2 {
		t.Fatal("completed retry duplicated")
	}
	claim, err := f.print.Claim(ctx, f.agent)
	check(t, err)
	if claim.Job.OriginType != "automation" {
		t.Fatal("not a normal automation job")
	}
	data, err := f.print.AgentDownload(ctx, f.agent, claim.Job.ID, claim.LeaseToken)
	check(t, err)
	if string(data) != string(f.pdf) {
		t.Fatal("wrong artifact")
	}
	_, err = f.print.Report(ctx, f.agent, claim.Job.ID, claim.LeaseToken, "printing", "", "")
	check(t, err)
	_, err = f.print.Report(ctx, f.agent, claim.Job.ID, claim.LeaseToken, "completed", "", "")
	check(t, err)
	_, _, err = f.print.CreateQuickJob(ctx, f.p, printing.CreateJobInput{PrinterID: f.printer, AssetID: f.asset, Copies: 1, IdempotencyKey: "manual"})
	check(t, err)
	report, err := f.s.Report(ctx, f.p)
	check(t, err)
	if len(report) != 2 {
		t.Fatalf("report=%+v", report)
	}
	for _, m := range report {
		if m.Origin == "automatic" && (m.Jobs != 2 || m.Copies != 4 || m.Completed != 1) {
			t.Fatalf("automatic metrics=%+v", m)
		}
	}
	if f.count(t, "inventory_transactions") != 0 || f.count(t, "inventory_balances") != 0 {
		t.Fatal("inventory changed")
	}
	if f.count(t, "audit_logs") < 6 {
		t.Fatal("missing audit")
	}
}
func TestScheduleConcurrentMaterializationAndDisabled(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	r := f.rule(t, "scheduled")
	due := time.Now().Add(-time.Minute)
	sql(t, f.db, `UPDATE automation_rules SET next_run_at=$2 WHERE id=$1`, r.ID, due)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := f.s.materializeSchedule(ctx); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		check(t, e)
	}
	if f.count(t, "automation_executions") != 1 {
		t.Fatal("duplicate occurrence")
	}
	// Disable between materialization and execution: queued work is visibly skipped.
	in := r.RuleInput
	in.Enabled = false
	_, err := f.s.SaveRule(ctx, f.p, r.ID, in)
	check(t, err)
	check(t, f.s.Tick(ctx))
	if f.count(t, "printer_jobs") != 0 {
		t.Fatal("disabled rule printed")
	}
	runs, err := f.s.Runs(ctx, f.p, "", false)
	check(t, err)
	if runs[0].Status != "skipped" {
		t.Fatal(runs)
	}
	sql(t, f.db, `UPDATE automation_rules SET next_run_at=$2 WHERE id=$1`, r.ID, due)
	ok, err := f.s.materializeSchedule(ctx)
	check(t, err)
	if ok {
		t.Fatal("disabled materialized")
	}
}
func TestFailuresBackoffPauseRetryAndDailyLimit(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	r := f.rule(t, "ecommerce_batch_ready")
	id := f.test(t, r, "offline")
	sql(t, f.db, `UPDATE registered_printers SET status='offline' WHERE id=$1`, f.printer)
	for n := 1; n <= 3; n++ {
		check(t, f.s.Tick(ctx))
		run := scalar[string](t, f.db, `SELECT status FROM automation_executions WHERE id=$1`, id)
		if run != "failed" {
			t.Fatal(run)
		}
		c, e := f.s.claim(ctx)
		if !errors.Is(e, pgx.ErrNoRows) {
			t.Fatalf("backoff claimed %+v %v", c, e)
		}
		if n < 3 {
			sql(t, f.db, `UPDATE automation_rules SET backoff_until=now()-interval '1 second' WHERE id=$1`, r.ID)
			sql(t, f.db, `UPDATE automation_executions SET available_at=now()-interval '1 second' WHERE id=$1`, id)
		}
	}
	r, err := f.s.Rule(ctx, f.p, r.ID)
	check(t, err)
	if !r.Paused || r.ConsecutiveFailures != 3 {
		t.Fatal(r)
	}
	if !errors.Is(f.s.Retry(ctx, f.p, id), ErrConflict) {
		t.Fatal("paused retry allowed")
	}
	sql(t, f.db, `UPDATE registered_printers SET status='online',last_seen_at=now() WHERE id=$1`, f.printer)
	r, err = f.s.Pause(ctx, f.p, r.ID, r.Version, false)
	check(t, err)
	check(t, f.s.Retry(ctx, f.p, id))
	check(t, f.s.Tick(ctx))
	check(t, f.s.Retry(ctx, f.p, id))
	if f.count(t, "printer_jobs") != 1 {
		t.Fatal("retry did not make one job")
	}
	limit := 4
	in := r.RuleInput
	in.DailyLimit = &limit
	r, err = f.s.SaveRule(ctx, f.p, r.ID, in)
	check(t, err)
	f.test(t, r, "limit-1")
	f.test(t, r, "limit-2")
	f.test(t, r, "limit-3")
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for n := 0; n < 4; n++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- f.s.Tick(ctx) }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		check(t, e)
	}
	if copies := scalar[int](t, f.db, `SELECT sum(copies) FROM printer_jobs`); copies != 4 {
		t.Fatalf("daily copies=%d", copies)
	}
	if skipped := scalar[int](t, f.db, `SELECT count(*) FROM automation_executions WHERE status='skipped'`); skipped != 2 {
		t.Fatalf("skipped=%d", skipped)
	}
	history, err := f.s.History(ctx, f.p, r.ID)
	check(t, err)
	found := false
	for _, h := range history {
		if h.Action == "automation.auto_paused" {
			found = true
		}
	}
	if !found {
		t.Fatal("no auto-pause audit")
	}
}
func TestTenantPermissionsVersionsAndTimezone(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	r := f.rule(t, "scheduled")
	if _, e := f.s.Rule(ctx, f.other, r.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
	if _, e := f.s.TestRun(ctx, f.other, r.ID, "cross"); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
	if _, e := f.s.SaveRule(ctx, f.other, "", r.RuleInput); !errors.Is(e, ErrInvalidInput) {
		t.Fatal(e)
	}
	other, err := f.s.Rules(ctx, f.other)
	check(t, err)
	if len(other) != 0 {
		t.Fatal("cross tenant rules")
	}
	for _, permission := range []string{"automations.view", "automations.manage"} {
		sql(t, f.db, `DELETE FROM role_permissions WHERE company_id=$1 AND permission_key=$2`, f.p.CompanyID, permission)
	}
	if _, e := f.s.Rules(ctx, f.p); !errors.Is(e, authorization.ErrPermissionDenied) {
		t.Fatal(e)
	}
	if _, e := f.s.SaveRule(ctx, f.p, r.ID, r.RuleInput); !errors.Is(e, authorization.ErrPermissionDenied) {
		t.Fatal(e)
	}
	if _, e := f.s.TestRun(ctx, f.p, r.ID, "denied"); !errors.Is(e, authorization.ErrPermissionDenied) {
		t.Fatal(e)
	}
	sql(t, f.db, `INSERT INTO role_permissions(company_id,role_id,permission_key) SELECT $1,$2,key FROM permissions WHERE key LIKE 'automations.%'`, f.p.CompanyID, f.role)
	check(t, f.s.SetTimezone(ctx, f.p, "Asia/Kolkata"))
	old, err := f.s.Rule(ctx, f.p, r.ID)
	check(t, err)
	if old.Timezone != "UTC" {
		t.Fatal("silently rewrote existing timezone")
	}
	updated, err := f.s.SaveRule(ctx, f.p, r.ID, r.RuleInput)
	check(t, err)
	if updated.Timezone != "Asia/Kolkata" || updated.Version != 2 {
		t.Fatal(updated)
	}
	if _, e := f.s.SaveRule(ctx, f.p, r.ID, r.RuleInput); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	preview, err := f.s.Preview(ctx, f.p, r.Schedule)
	check(t, err)
	if len(preview) != 10 || !preview[0].Equal(*updated.NextRunAt) {
		t.Fatalf("preview mismatch %v %v", preview, updated.NextRunAt)
	}
	if e := f.s.SetTimezone(ctx, f.p, "Local"); !errors.Is(e, ErrInvalidInput) {
		t.Fatal(e)
	}
	upcoming, err := f.s.Upcoming(ctx, f.other)
	check(t, err)
	report, err := f.s.Report(ctx, f.other)
	check(t, err)
	if len(upcoming) != 0 || len(report) != 0 {
		t.Fatal("tenant leak")
	}
}
func TestDomainEventDurabilityMatchingAndRollback(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	for _, kind := range []string{"ecommerce_batch_ready", "consignment_packing", "consignment_packed"} {
		f.rule(t, kind)
	}
	source := scalar[string](t, f.db, `SELECT gen_random_uuid()`)
	tx, err := f.db.Begin(ctx)
	check(t, err)
	check(t, domainevent.Record(ctx, tx, f.p.CompanyID, f.p.UserID, "ecommerce_batch_ready", source, 1))
	check(t, tx.Rollback(ctx))
	if f.count(t, "automation_domain_events") != 0 {
		t.Fatal("rolled back event survived")
	}
	for _, kind := range []string{"ecommerce_batch_ready", "consignment_packing", "consignment_packed"} {
		tx, err = f.db.Begin(ctx)
		check(t, err)
		check(t, domainevent.Record(ctx, tx, f.p.CompanyID, f.p.UserID, kind, source, 1))
		check(t, domainevent.Record(ctx, tx, f.p.CompanyID, f.p.UserID, kind, source, 1))
		check(t, tx.Commit(ctx))
	}
	check(t, f.s.Tick(ctx))
	check(t, f.s.Tick(ctx))
	if f.count(t, "printer_jobs") != 3 || f.count(t, "automation_domain_events") != 3 {
		t.Fatal("event replay or matching failed")
	}
	runs, err := f.s.Runs(ctx, f.other, "", false)
	check(t, err)
	if len(runs) != 0 {
		t.Fatal("event tenant leak")
	}
	// Newly enabled rules must not act on an old event.
	tx, err = f.db.Begin(ctx)
	check(t, err)
	check(t, domainevent.Record(ctx, tx, f.p.CompanyID, f.p.UserID, "consignment_packed", source, 2))
	check(t, tx.Commit(ctx))
	f.rule(t, "consignment_packed")
	check(t, f.s.Tick(ctx))
	if f.count(t, "printer_jobs") != 4 {
		t.Fatal("new rule matched historical event")
	}
}

type crashQueue struct{ actual *printing.Service }

func (q crashQueue) QueueAutomationTx(ctx context.Context, tx pgx.Tx, p auth.Principal, id, printer, asset string, copies int) (printing.Job, error) {
	job, err := q.actual.QueueAutomationTx(ctx, tx, p, id, printer, asset, copies)
	if err != nil {
		return job, err
	}
	return job, errors.New("simulated interruption after Printing insertion")
}
func TestJobAndExecutionCommitAtomicallyAfterCrash(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	r := f.rule(t, "ecommerce_batch_ready")
	id := f.test(t, r, "crash")
	c, err := f.s.claim(ctx)
	check(t, err)
	f.s.printing = crashQueue{actual: f.print}
	if err = f.s.execute(ctx, c); err == nil {
		t.Fatal("expected simulated interruption")
	}
	if f.count(t, "printer_jobs") != 0 || f.count(t, "printer_job_events") != 0 {
		t.Fatal("partial job survived transaction rollback")
	}
	f.s.printing = f.print
	sql(t, f.db, `UPDATE automation_executions SET lease_expires_at=now()-interval '1 second' WHERE id=$1`, id)
	check(t, f.s.Tick(ctx))
	check(t, f.s.Tick(ctx))
	if f.count(t, "printer_jobs") != 1 {
		t.Fatal("restart failed exact-once insertion")
	}
}
func TestMigrationRoundTripAndHistoricalJobGuard(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	down, err := os.ReadFile("../../migrations/000022_printing_automation.down.sql")
	check(t, err)
	up, err := os.ReadFile("../../migrations/000022_printing_automation.up.sql")
	check(t, err)
	tx, err := f.db.Begin(ctx)
	check(t, err)
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, string(down))
	check(t, err)
	var exists *string
	check(t, tx.QueryRow(ctx, `SELECT to_regclass(current_schema()||'.automation_rules')`).Scan(&exists))
	if exists != nil {
		t.Fatal("down left automation table")
	}
	_, err = tx.Exec(ctx, string(up))
	check(t, err)
	check(t, tx.Commit(ctx))
	sql(t, f.db, `INSERT INTO role_permissions(company_id,role_id,permission_key) SELECT $1,$2,key FROM permissions WHERE key LIKE 'automations.%'`, f.p.CompanyID, f.role)
	r := f.rule(t, "ecommerce_batch_ready")
	f.test(t, r, "history")
	check(t, f.s.Tick(ctx))
	tx, err = f.db.Begin(ctx)
	check(t, err)
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, string(down)); err == nil {
		t.Fatal("downgrade removed automation job history")
	}
}

func TestSchedulerLifecycleCancellation(t *testing.T) {
	f := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { defer close(done); f.s.Run(ctx, slog.New(slog.NewTextHandler(io.Discard, nil))) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not stop on cancellation")
	}
}
func TestInactiveAssetAndPhysicalFailureRemainVisible(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	r := f.rule(t, "ecommerce_batch_ready")
	f.test(t, r, "inactive-asset")
	sql(t, f.db, `UPDATE print_library_assets SET active=false WHERE id=$1`, f.asset)
	check(t, f.s.Tick(ctx))
	runs, err := f.s.Runs(ctx, f.p, r.ID, true)
	check(t, err)
	if len(runs) != 1 || runs[0].Status != "failed" || runs[0].Error == nil {
		t.Fatalf("inactive asset runs=%+v", runs)
	}
	sql(t, f.db, `UPDATE print_library_assets SET active=true WHERE id=$1`, f.asset)
	sql(t, f.db, `UPDATE automation_rules SET backoff_until=NULL WHERE id=$1`, r.ID)
	check(t, f.s.Retry(ctx, f.p, runs[0].ID))
	check(t, f.s.Tick(ctx))
	claim, err := f.print.Claim(ctx, f.agent)
	check(t, err)
	_, err = f.print.Report(ctx, f.agent, claim.Job.ID, claim.LeaseToken, "failed", "paper_out", "Printer needs paper")
	check(t, err)
	runs, err = f.s.Runs(ctx, f.p, r.ID, true)
	check(t, err)
	if len(runs) != 1 || runs[0].Status != "completed" || runs[0].JobStatus == nil || *runs[0].JobStatus != "failed" {
		t.Fatalf("physical failure=%+v", runs)
	}
	check(t, f.s.Retry(ctx, f.p, runs[0].ID))
	check(t, f.s.Tick(ctx))
	if f.count(t, "printer_jobs") != 1 {
		t.Fatal("automation retried physical failure")
	}
	metrics, err := f.s.Report(ctx, f.p)
	check(t, err)
	if len(metrics) != 1 || metrics[0].Failed != 1 || metrics[0].FailureEvents != 1 {
		t.Fatal(metrics)
	}
}

func TestCompanyEditLockAllowsExecutionForeignKeys(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	r := f.rule(t, "ecommerce_batch_ready")
	f.test(t, r, "lock-order")
	c, err := f.s.claim(ctx)
	check(t, err)
	// SaveRule/SetTimezone serialize on this exact non-key tenant lock. While
	// it is held, a worker must still insert its tenant-scoped job/audit rows.
	tx, err := f.db.Begin(ctx)
	check(t, err)
	defer tx.Rollback(ctx)
	var zone string
	check(t, tx.QueryRow(ctx, `SELECT timezone FROM companies WHERE id=$1 FOR NO KEY UPDATE`, f.p.CompanyID).Scan(&zone))
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	check(t, f.s.execute(bounded, c))
	check(t, tx.Rollback(ctx))
	if f.count(t, "printer_jobs") != 1 {
		t.Fatal("execution blocked by company edit lock")
	}
}
