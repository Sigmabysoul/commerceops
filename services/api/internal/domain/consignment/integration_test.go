// This file contains focused regression tests for the behavior owned by this package in the consignment package.
package consignment

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/domain/inventory"
	"github.com/commerceops/commerceops/services/api/internal/domain/reporting"
	"github.com/commerceops/commerceops/services/api/internal/domain/traceability"
	"github.com/commerceops/commerceops/services/api/internal/platform/auth"
	"github.com/commerceops/commerceops/services/api/internal/platform/authorization"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	db                                                                                                                                    *pgxpool.Pool
	service                                                                                                                               *Service
	inventory                                                                                                                             *inventory.Service
	principal, worker                                                                                                                     auth.Principal
	company, otherCompany, role, workerRole, employee, workerEmployee, product, secondProduct, otherProduct, department, secondDepartment string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{db: db}
	suffix := fmt.Sprint(time.Now().UnixNano())
	mustScan(t, db, `INSERT INTO companies(name) VALUES($1) RETURNING id`, []any{"Consignment A " + suffix}, &f.company)
	mustScan(t, db, `INSERT INTO companies(name) VALUES($1) RETURNING id`, []any{"Consignment B " + suffix}, &f.otherCompany)
	var user, workerUser string
	mustScan(t, db, `INSERT INTO users(email,password_hash) VALUES($1,'test') RETURNING id`, []any{"consignment-owner-" + suffix + "@example.test"}, &user)
	mustScan(t, db, `INSERT INTO users(email,password_hash) VALUES($1,'test') RETURNING id`, []any{"consignment-worker-" + suffix + "@example.test"}, &workerUser)
	mustExec(t, db, `INSERT INTO company_users(company_id,user_id) VALUES($1,$2),($1,$3),($4,$2)`, f.company, user, workerUser, f.otherCompany)
	mustScan(t, db, `INSERT INTO employees(company_id,user_id,display_name) VALUES($1,$2,'Owner') RETURNING id`, []any{f.company, user}, &f.employee)
	mustScan(t, db, `INSERT INTO employees(company_id,user_id,display_name) VALUES($1,$2,'Worker') RETURNING id`, []any{f.company, workerUser}, &f.workerEmployee)
	mustScan(t, db, `INSERT INTO roles(company_id,name) VALUES($1,'Consignment Manager') RETURNING id`, []any{f.company}, &f.role)
	mustScan(t, db, `INSERT INTO roles(company_id,name) VALUES($1,'Department Worker') RETURNING id`, []any{f.company}, &f.workerRole)
	mustExec(t, db, `INSERT INTO role_permissions(company_id,role_id,permission_key) VALUES ($1,$2,'consignments.view'),($1,$2,'consignments.work'),($1,$2,'consignments.manage'),($1,$2,'consignments.outbound'),($1,$2,'inventory.view'),($1,$2,'inventory.stock_in'),($1,$2,'reports.view'),($1,$2,'traceability.view'),($1,$2,'traceability.manage'),($1,$3,'consignments.work'),($1,$3,'reports.view')`, f.company, f.role, f.workerRole)
	mustExec(t, db, `INSERT INTO company_user_roles(company_id,user_id,role_id) VALUES($1,$2,$3),($1,$4,$5)`, f.company, user, f.role, workerUser, f.workerRole)
	mustExec(t, db, `INSERT INTO module_entitlements(company_id,module_key,enabled) VALUES($1,'inventory',true),($1,'consignments',true),($1,'traceability',true)`, f.company)
	mustScan(t, db, `INSERT INTO products(company_id,internal_code,name) VALUES($1,'CON-A','Consignment A') RETURNING id`, []any{f.company}, &f.product)
	mustScan(t, db, `INSERT INTO products(company_id,internal_code,name) VALUES($1,'CON-B','Consignment B') RETURNING id`, []any{f.company}, &f.secondProduct)
	mustScan(t, db, `INSERT INTO products(company_id,internal_code,name) VALUES($1,'OTHER','Other tenant') RETURNING id`, []any{f.otherCompany}, &f.otherProduct)
	f.principal = auth.Principal{CompanyID: f.company, UserID: user}
	f.worker = auth.Principal{CompanyID: f.company, UserID: workerUser}
	authorizer := authorization.NewService(db)
	f.inventory = inventory.NewService(db, authorizer)
	f.service = NewService(db, authorizer, f.inventory)
	d1, err := f.service.CreateDepartment(ctx, f.principal, DepartmentInput{Name: "GB"})
	if err != nil {
		t.Fatal(err)
	}
	f.department = d1.ID
	d2, err := f.service.CreateDepartment(ctx, f.principal, DepartmentInput{Name: "Synthetic Team"})
	if err != nil {
		t.Fatal(err)
	}
	f.secondDepartment = d2.ID
	mustExec(t, db, `INSERT INTO product_department_assignments(company_id,product_id,department_id,assigned_by) VALUES($1,$2,$3,$4),($1,$5,$6,$4)`, f.company, f.product, f.department, user, f.secondProduct, f.secondDepartment)
	if _, err = f.service.SetDepartmentMembers(ctx, f.principal, f.department, MembershipInput{EmployeeIDs: []string{f.workerEmployee}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return f
}

func TestPhaseNineLifecycleInventoryAuditAndReporting(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, _, err := f.inventory.StockIn(ctx, f.principal, inventory.CommandInput{ProductID: f.product, Quantity: 10, Reason: "Consignment stock", IdempotencyKey: "p9-stock"}); err != nil {
		t.Fatal(err)
	}
	item, replay, err := f.service.Create(ctx, f.principal, CreateInput{OrderReference: "SO-100", DealerReference: stringPtr("DEALER-1"), PouchReference: stringPtr("POUCH-7"), SourceType: "import", SourceReference: stringPtr("sanitized-so.csv:row-2"), Lines: []LineInput{{ProductID: f.product, DepartmentID: f.department, RequiredQuantity: 4}}, IdempotencyKey: strings.Repeat("c", 128)})
	if err != nil || replay || item.Status != "created" || item.Lines[0].ProductID != f.product {
		t.Fatalf("create=%#v replay=%v err=%v", item, replay, err)
	}
	item, _, err = f.service.Allocate(ctx, f.principal, item.ID, ActionInput{ExpectedVersion: item.Version, IdempotencyKey: "p9-allocate"})
	if err != nil || item.Status != "allocated" {
		t.Fatalf("allocate=%#v err=%v", item, err)
	}
	assertBalance(t, f, 10, 4, 6)
	item, _, err = f.service.Transition(ctx, f.principal, item.ID, TransitionInput{TargetStatus: "picking", ExpectedVersion: item.Version, IdempotencyKey: "p9-picking"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.UpdateProgress(ctx, f.principal, item.ID, item.Lines[0].ID, ProgressInput{ReadyQuantity: 2, PackedQuantity: 0, ExpectedVersion: item.Lines[0].Version, IdempotencyKey: "p9-partial"})
	if err != nil || item.Lines[0].Progress != "pending" {
		t.Fatalf("partial=%#v err=%v", item, err)
	}
	if _, _, err = f.service.Transition(ctx, f.principal, item.ID, TransitionInput{TargetStatus: "ready", ExpectedVersion: item.Version, IdempotencyKey: "p9-ready-too-soon"}); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("incomplete ready=%v", err)
	}
	item, _, err = f.service.UpdateProgress(ctx, f.principal, item.ID, item.Lines[0].ID, ProgressInput{ReadyQuantity: 4, PackedQuantity: 0, ExpectedVersion: item.Lines[0].Version, IdempotencyKey: "p9-ready-progress"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.Transition(ctx, f.principal, item.ID, TransitionInput{TargetStatus: "ready", ExpectedVersion: item.Version, IdempotencyKey: "p9-ready"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.Transition(ctx, f.principal, item.ID, TransitionInput{TargetStatus: "packing", ExpectedVersion: item.Version, IdempotencyKey: "p9-packing"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.UpdateProgress(ctx, f.principal, item.ID, item.Lines[0].ID, ProgressInput{ReadyQuantity: 4, PackedQuantity: 4, ExpectedVersion: item.Lines[0].Version, IdempotencyKey: "p9-packed-progress"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.Transition(ctx, f.principal, item.ID, TransitionInput{TargetStatus: "packed", ExpectedVersion: item.Version, IdempotencyKey: "p9-packed"})
	if err != nil {
		t.Fatal(err)
	}
	outInput := ActionInput{ExpectedVersion: item.Version, IdempotencyKey: "p9-outbound"}
	item, replay, err = f.service.ConfirmOutbound(ctx, f.principal, item.ID, outInput)
	if err != nil || replay || item.Status != "outbound" {
		t.Fatalf("outbound=%#v replay=%v err=%v", item, replay, err)
	}
	assertBalance(t, f, 6, 0, 6)
	item, replay, err = f.service.ConfirmOutbound(ctx, f.principal, item.ID, outInput)
	if err != nil || !replay {
		t.Fatalf("outbound replay=%v err=%v", replay, err)
	}
	item, _, err = f.service.Transition(ctx, f.principal, item.ID, TransitionInput{TargetStatus: "completed", ExpectedVersion: item.Version, IdempotencyKey: "p9-complete"})
	if err != nil || item.CompletedAt == nil {
		t.Fatalf("complete=%#v err=%v", item, err)
	}
	var ledger, reservations, audits int
	mustScan(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1 AND transaction_type='consignment_out' AND reference_id=$2`, []any{f.company, item.ID}, &ledger)
	mustScan(t, f.db, `SELECT count(*) FROM inventory_reservations WHERE company_id=$1 AND source_id=$2 AND status='released' AND release_reason='Consumed by consignment outbound'`, []any{f.company, item.ID}, &reservations)
	mustScan(t, f.db, `SELECT count(*) FROM audit_logs WHERE company_id=$1 AND target_id=$2 AND action LIKE 'consignment.%'`, []any{f.company, item.ID}, &audits)
	if ledger != 1 || reservations != 1 || audits < 8 {
		t.Fatalf("ledger=%d reservations=%d audits=%d", ledger, reservations, audits)
	}
	report, err := reporting.NewService(f.db, authorization.NewService(f.db)).Dashboard(ctx, f.principal, reporting.Filter{From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), Marketplace: "flipkart", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if !report.ConsignmentAccess || report.Consignment == nil || report.Consignment.Completed != 1 || report.Consignment.InventoryOut != 4 || report.Inventory == nil || report.Inventory.ConsignmentOut != 4 {
		t.Fatalf("report=%#v inventory=%#v", report.Consignment, report.Inventory)
	}
	if _, err = f.db.Exec(ctx, `UPDATE consignment_events SET notes='rewrite' WHERE company_id=$1 AND consignment_id=$2`, f.company, item.ID); err == nil {
		t.Fatal("immutable consignment event was updated")
	}
}

func TestCancellationDepartmentIsolationAndConcurrentProgress(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	for n, p := range []string{f.product, f.secondProduct} {
		if _, _, err := f.inventory.StockIn(ctx, f.principal, inventory.CommandInput{ProductID: p, Quantity: 5, Reason: "Stock", IdempotencyKey: fmt.Sprintf("cancel-stock-%d", n)}); err != nil {
			t.Fatal(err)
		}
	}
	visible, _, err := f.service.Create(ctx, f.principal, CreateInput{OrderReference: "SO-WORKER", PouchReference: stringPtr("REUSED-POUCH"), SourceType: "manual", Lines: []LineInput{{ProductID: f.product, DepartmentID: f.department, RequiredQuantity: 2}}, IdempotencyKey: "worker-visible"})
	if err != nil {
		t.Fatal(err)
	}
	hidden, _, err := f.service.Create(ctx, f.principal, CreateInput{OrderReference: "SO-HIDDEN", PouchReference: stringPtr("REUSED-POUCH"), SourceType: "manual", Lines: []LineInput{{ProductID: f.secondProduct, DepartmentID: f.secondDepartment, RequiredQuantity: 2}}, IdempotencyKey: "worker-hidden"})
	if err != nil {
		t.Fatal(err)
	}
	workerItems, err := f.service.List(ctx, f.worker, Filter{})
	if err != nil || len(workerItems) != 1 || workerItems[0].ID != visible.ID {
		t.Fatalf("worker list=%#v err=%v", workerItems, err)
	}
	if len(workerItems[0].Events) != 0 {
		t.Fatalf("department worker received broad event history=%#v", workerItems[0].Events)
	}
	if _, err = f.service.Get(ctx, f.worker, hidden.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("hidden detail=%v", err)
	}
	if _, _, err = f.service.Create(ctx, f.worker, CreateInput{OrderReference: "SO-DENIED", SourceType: "manual", Lines: []LineInput{{ProductID: f.product, DepartmentID: f.department, RequiredQuantity: 1}}, IdempotencyKey: "worker-create-denied"}); !errors.Is(err, authorization.ErrPermissionDenied) {
		t.Fatalf("department worker created consignment=%v", err)
	}
	workerReport, err := reporting.NewService(f.db, authorization.NewService(f.db)).Dashboard(ctx, f.worker, reporting.Filter{From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), Limit: 50})
	if err != nil || workerReport.Consignment == nil || len(workerReport.Consignment.Products) != 1 || workerReport.Consignment.Products[0].ProductID != f.product || workerReport.Consignment.InventoryOut != 0 {
		t.Fatalf("department report=%#v err=%v", workerReport.Consignment, err)
	}
	visible, _, err = f.service.Allocate(ctx, f.principal, visible.ID, ActionInput{ExpectedVersion: visible.Version, IdempotencyKey: "worker-allocate"})
	if err != nil {
		t.Fatal(err)
	}
	visible, _, err = f.service.Transition(ctx, f.principal, visible.ID, TransitionInput{TargetStatus: "picking", ExpectedVersion: visible.Version, IdempotencyKey: "worker-picking"})
	if err != nil {
		t.Fatal(err)
	}
	line := visible.Lines[0]
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for n := 1; n <= 2; n++ {
		wg.Add(1)
		go func(quantity int64) {
			defer wg.Done()
			<-start
			_, _, runErr := f.service.UpdateProgress(ctx, f.worker, visible.ID, line.ID, ProgressInput{ReadyQuantity: quantity, PackedQuantity: 0, ExpectedVersion: line.Version, IdempotencyKey: fmt.Sprintf("worker-progress-%d", quantity)})
			results <- runErr
		}(int64(n))
	}
	close(start)
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if errors.Is(e, ErrConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("concurrent success=%d conflict=%d", success, conflict)
	}
	visible, err = f.service.Get(ctx, f.principal, visible.ID)
	if err != nil {
		t.Fatal(err)
	}
	visible, _, err = f.service.Cancel(ctx, f.principal, visible.ID, ActionInput{ExpectedVersion: visible.Version, IdempotencyKey: "worker-cancel", Notes: stringPtr("Dealer withdrew order")})
	if err != nil || visible.Status != "cancelled" {
		t.Fatalf("cancel=%#v err=%v", visible, err)
	}
	assertProductBalance(t, f, f.product, 5, 0, 5)
	var outbound int
	mustScan(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1 AND transaction_type='consignment_out' AND reference_id=$2`, []any{f.company, visible.ID}, &outbound)
	if outbound != 0 {
		t.Fatalf("cancel created outbound=%d", outbound)
	}
	if _, _, err = f.service.Create(ctx, f.principal, CreateInput{OrderReference: "SO-CROSS", SourceType: "manual", Lines: []LineInput{{ProductID: f.otherProduct, DepartmentID: f.department, RequiredQuantity: 1}}, IdempotencyKey: "cross-product"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("cross tenant product=%v", err)
	}
}

func TestConcurrentDifferentProductLinesBothCommit(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	for index, productID := range []string{f.product, f.secondProduct} {
		if _, _, err := f.inventory.StockIn(ctx, f.principal, inventory.CommandInput{ProductID: productID, Quantity: 3, Reason: "Concurrent line stock", IdempotencyKey: fmt.Sprintf("multi-stock-%d", index)}); err != nil {
			t.Fatal(err)
		}
	}
	item, _, err := f.service.Create(ctx, f.principal, CreateInput{OrderReference: "SO-MULTI", SourceType: "manual", Lines: []LineInput{{ProductID: f.product, DepartmentID: f.department, RequiredQuantity: 2}, {ProductID: f.secondProduct, DepartmentID: f.secondDepartment, RequiredQuantity: 3}}, IdempotencyKey: "multi-create"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.Allocate(ctx, f.principal, item.ID, ActionInput{ExpectedVersion: item.Version, IdempotencyKey: "multi-allocate"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.Transition(ctx, f.principal, item.ID, TransitionInput{TargetStatus: "picking", ExpectedVersion: item.Version, IdempotencyKey: "multi-picking"})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, len(item.Lines))
	var wg sync.WaitGroup
	for index, line := range item.Lines {
		wg.Add(1)
		go func(index int, line Line) {
			defer wg.Done()
			<-start
			_, _, runErr := f.service.UpdateProgress(ctx, f.principal, item.ID, line.ID, ProgressInput{ReadyQuantity: line.RequiredQuantity, ExpectedVersion: line.Version, IdempotencyKey: fmt.Sprintf("multi-progress-%d", index)})
			results <- runErr
		}(index, line)
	}
	close(start)
	wg.Wait()
	close(results)
	for runErr := range results {
		if runErr != nil {
			t.Fatal(runErr)
		}
	}
	item, err = f.service.Get(ctx, f.principal, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range item.Lines {
		if line.ReadyQuantity != line.RequiredQuantity {
			t.Fatalf("line not ready=%#v", line)
		}
	}
}

func TestPhaseNineMigrationUpDown(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	schema := "p9_migration_" + fmt.Sprint(time.Now().UnixNano())
	mustExec(t, tx, `CREATE SCHEMA `+schema)
	mustExec(t, tx, `SET LOCAL search_path TO `+schema+`,public`)
	root := filepath.Join("..", "..", "..", "migrations")
	for n := 1; n <= 17; n++ {
		matches, e := filepath.Glob(filepath.Join(root, fmt.Sprintf("%06d_*.up.sql", n)))
		if e != nil || len(matches) != 1 {
			t.Fatalf("migration %d matches=%v err=%v", n, matches, e)
		}
		data, e := os.ReadFile(matches[0])
		if e != nil {
			t.Fatal(e)
		}
		if _, e = tx.Exec(ctx, string(data)); e != nil {
			t.Fatalf("apply %s: %v", matches[0], e)
		}
	}
	var exists *string
	if err = tx.QueryRow(ctx, `SELECT to_regclass($1)`, schema+".consignments").Scan(&exists); err != nil || exists == nil {
		t.Fatalf("up=%v err=%v", exists, err)
	}
	down, err := os.ReadFile(filepath.Join(root, "000017_consignment_management.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	exists = nil
	if err = tx.QueryRow(ctx, `SELECT to_regclass($1)`, schema+".consignments").Scan(&exists); err != nil || exists != nil {
		t.Fatalf("down=%v err=%v", exists, err)
	}
}

func assertBalance(t *testing.T, f *fixture, onHand, reserved, available int64) {
	assertProductBalance(t, f, f.product, onHand, reserved, available)
}
func assertProductBalance(t *testing.T, f *fixture, product string, onHand, reserved, available int64) {
	t.Helper()
	var a, b, c int64
	mustScan(t, f.db, `SELECT on_hand,reserved,on_hand-reserved FROM inventory_balances WHERE company_id=$1 AND product_id=$2`, []any{f.company, product}, &a, &b, &c)
	if a != onHand || b != reserved || c != available {
		t.Fatalf("balance=%d/%d/%d want %d/%d/%d", a, b, c, onHand, reserved, available)
	}
}
func mustExec(t *testing.T, db interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, sql string, args ...any) {
	t.Helper()
	if _, err := db.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}
func mustScan(t *testing.T, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, sql string, args []any, dest ...any) {
	t.Helper()
	if err := db.QueryRow(context.Background(), sql, args...).Scan(dest...); err != nil {
		t.Fatal(err)
	}
}
func stringPtr(v string) *string { return &v }

func TestPackingAutomationEventsFollowVersionedTransitions(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_, _, err := f.inventory.StockIn(ctx, f.principal, inventory.CommandInput{ProductID: f.product, Quantity: 4, Reason: "Automation hook fixture", IdempotencyKey: "automation-stock"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err := f.service.Create(ctx, f.principal, CreateInput{OrderReference: "AUTOMATION", SourceType: "manual", Lines: []LineInput{{ProductID: f.product, DepartmentID: f.department, RequiredQuantity: 2}}, IdempotencyKey: "automation-create"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.Allocate(ctx, f.principal, item.ID, ActionInput{ExpectedVersion: item.Version, IdempotencyKey: "automation-allocate"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.Transition(ctx, f.principal, item.ID, TransitionInput{TargetStatus: "picking", ExpectedVersion: item.Version, IdempotencyKey: "automation-picking"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.UpdateProgress(ctx, f.principal, item.ID, item.Lines[0].ID, ProgressInput{ReadyQuantity: 2, PackedQuantity: 0, ExpectedVersion: item.Lines[0].Version, IdempotencyKey: "automation-ready-progress"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.Transition(ctx, f.principal, item.ID, TransitionInput{TargetStatus: "ready", ExpectedVersion: item.Version, IdempotencyKey: "automation-ready"})
	if err != nil {
		t.Fatal(err)
	}
	packing := TransitionInput{TargetStatus: "packing", ExpectedVersion: item.Version, IdempotencyKey: "automation-packing"}
	for n := 0; n < 2; n++ {
		item, _, err = f.service.Transition(ctx, f.principal, item.ID, packing)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err = f.service.Transition(ctx, f.principal, item.ID, TransitionInput{TargetStatus: "packed", ExpectedVersion: item.Version, IdempotencyKey: "automation-invalid-packed"}); !errors.Is(err, ErrIncomplete) {
		t.Fatal(err)
	}
	var count int
	mustScan(t, f.db, `SELECT count(*) FROM automation_domain_events WHERE company_id=$1 AND source_id=$2`, []any{f.company, item.ID}, &count)
	if count != 1 {
		t.Fatalf("events=%d", count)
	}
	item, _, err = f.service.UpdateProgress(ctx, f.principal, item.ID, item.Lines[0].ID, ProgressInput{ReadyQuantity: 2, PackedQuantity: 2, ExpectedVersion: item.Lines[0].Version, IdempotencyKey: "automation-packed-progress"})
	if err != nil {
		t.Fatal(err)
	}
	packed := TransitionInput{TargetStatus: "packed", ExpectedVersion: item.Version, IdempotencyKey: "automation-packed"}
	for n := 0; n < 2; n++ {
		item, _, err = f.service.Transition(ctx, f.principal, item.ID, packed)
		if err != nil {
			t.Fatal(err)
		}
	}
	mustScan(t, f.db, `SELECT count(*) FROM automation_domain_events WHERE company_id=$1 AND source_id=$2 AND event_type IN ('consignment_packing','consignment_packed')`, []any{f.company, item.ID}, &count)
	if count != 2 {
		t.Fatalf("events=%d", count)
	}
	assertBalance(t, f, 4, 2, 2)
}

func TestPhase22MixedTraceabilityPackingAndOutbound(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	for index, productID := range []string{f.product, f.secondProduct} {
		if _, _, err := f.inventory.StockIn(ctx, f.principal, inventory.CommandInput{ProductID: productID, Quantity: 5, Reason: "Phase 22 stock", IdempotencyKey: fmt.Sprintf("p22-stock-%d", index)}); err != nil {
			t.Fatal(err)
		}
	}
	box := readyTraceBox(t, f, []traceability.ContentInput{{ProductID: f.product, Quantity: 2}, {ProductID: f.secondProduct, Quantity: 3}}, "p22-mixed-box")
	item, _, err := f.service.Create(ctx, f.principal, CreateInput{OrderReference: "P22-MIXED", SourceType: "manual", TraceabilityRequired: true, Lines: []LineInput{{ProductID: f.product, DepartmentID: f.department, RequiredQuantity: 2}, {ProductID: f.secondProduct, DepartmentID: f.secondDepartment, RequiredQuantity: 3}}, IdempotencyKey: "p22-create"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.Allocate(ctx, f.principal, item.ID, ActionInput{ExpectedVersion: item.Version, IdempotencyKey: "p22-allocate"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.Transition(ctx, f.principal, item.ID, TransitionInput{TargetStatus: "picking", ExpectedVersion: item.Version, IdempotencyKey: "p22-picking"})
	if err != nil {
		t.Fatal(err)
	}
	first, second := item.Lines[0], item.Lines[1]
	if _, _, err = f.service.LinkTraceBox(ctx, f.principal, item.ID, TraceLinkInput{LineID: first.ID, TraceBoxIdentifier: box.OpaqueIdentifier, Quantity: first.RequiredQuantity + 1, ExpectedVersion: item.Version, IdempotencyKey: "p22-overallocate"}); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("overallocate=%v", err)
	}
	linkInput := TraceLinkInput{LineID: first.ID, TraceBoxIdentifier: box.OpaqueIdentifier, Quantity: first.RequiredQuantity, ExpectedVersion: item.Version, IdempotencyKey: "p22-link-first"}
	item, replay, err := f.service.LinkTraceBox(ctx, f.principal, item.ID, linkInput)
	if err != nil || replay || currentLine(item, first.ID).TracedQuantity != first.RequiredQuantity {
		t.Fatalf("first link=%#v replay=%v err=%v", item.Lines, replay, err)
	}
	item, replay, err = f.service.LinkTraceBox(ctx, f.principal, item.ID, linkInput)
	if err != nil || !replay {
		t.Fatalf("link replay=%v err=%v", replay, err)
	}
	second = currentLine(item, second.ID)
	item, _, err = f.service.LinkTraceBox(ctx, f.principal, item.ID, TraceLinkInput{LineID: second.ID, TraceBoxIdentifier: box.OpaqueIdentifier, Quantity: second.RequiredQuantity, ExpectedVersion: item.Version, IdempotencyKey: "p22-link-second"})
	if err != nil {
		t.Fatal(err)
	}
	if len(item.DepartmentProgress) != 2 {
		t.Fatalf("department progress=%#v", item.DepartmentProgress)
	}
	first = currentLine(item, first.ID)
	item, _, err = f.service.UpdateProgress(ctx, f.principal, item.ID, first.ID, ProgressInput{ReadyQuantity: first.RequiredQuantity, ExpectedVersion: first.Version, IdempotencyKey: "p22-progress-first"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.service.Transition(ctx, f.principal, item.ID, TransitionInput{TargetStatus: "ready", ExpectedVersion: item.Version, IdempotencyKey: "p22-ready-early"}); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("early ready=%v", err)
	}
	second = currentLine(item, second.ID)
	item, _, err = f.service.UpdateProgress(ctx, f.principal, item.ID, second.ID, ProgressInput{ReadyQuantity: second.RequiredQuantity, ExpectedVersion: second.Version, IdempotencyKey: "p22-progress-second"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.RecordTraceEvidence(ctx, f.principal, item.ID, TraceEvidenceInput{EvidenceType: "pouch_reference", ReferenceValue: "REUSED-P22", TraceBoxIdentifier: &box.OpaqueIdentifier, ExpectedVersion: item.Version, IdempotencyKey: "p22-evidence"})
	if err != nil || len(item.TraceEvidence) != 1 {
		t.Fatalf("evidence=%#v err=%v", item.TraceEvidence, err)
	}
	item, _, err = f.service.Transition(ctx, f.principal, item.ID, TransitionInput{TargetStatus: "ready", ExpectedVersion: item.Version, IdempotencyKey: "p22-ready"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.Transition(ctx, f.principal, item.ID, TransitionInput{TargetStatus: "packing", ExpectedVersion: item.Version, IdempotencyKey: "p22-packing"})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range item.Lines {
		item, _, err = f.service.UpdateProgress(ctx, f.principal, item.ID, line.ID, ProgressInput{ReadyQuantity: line.RequiredQuantity, PackedQuantity: line.RequiredQuantity, ExpectedVersion: line.Version, IdempotencyKey: "p22-packed-" + line.ID})
		if err != nil {
			t.Fatal(err)
		}
	}
	item, _, err = f.service.Transition(ctx, f.principal, item.ID, TransitionInput{TargetStatus: "packed", ExpectedVersion: item.Version, IdempotencyKey: "p22-packed"})
	if err != nil {
		t.Fatal(err)
	}
	out := ActionInput{ExpectedVersion: item.Version, IdempotencyKey: "p22-outbound"}
	item, replay, err = f.service.ConfirmOutbound(ctx, f.principal, item.ID, out)
	if err != nil || replay {
		t.Fatalf("outbound replay=%v err=%v", replay, err)
	}
	_, replay, err = f.service.ConfirmOutbound(ctx, f.principal, item.ID, out)
	if err != nil || !replay {
		t.Fatalf("outbound replay=%v err=%v", replay, err)
	}
	assertProductBalance(t, f, f.product, 3, 0, 3)
	assertProductBalance(t, f, f.secondProduct, 2, 0, 2)
	other, _, err := f.service.Create(ctx, f.principal, CreateInput{OrderReference: "P22-EVIDENCE", SourceType: "manual", Lines: []LineInput{{ProductID: f.product, DepartmentID: f.department, RequiredQuantity: 1}}, IdempotencyKey: "p22-other"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = f.service.RecordTraceEvidence(ctx, f.principal, other.ID, TraceEvidenceInput{EvidenceType: "pouch_reference", ReferenceValue: "REUSED-P22", ExpectedVersion: other.Version, IdempotencyKey: "p22-other-evidence"})
	if err != nil {
		t.Fatalf("nonunique evidence=%v", err)
	}
}

func TestPhase22StaleBoxAndUnlinkGuards(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	box := readyTraceBox(t, f, []traceability.ContentInput{{ProductID: f.product, Quantity: 2}}, "p22-stale-box")
	item, _, err := f.service.Create(ctx, f.principal, CreateInput{OrderReference: "P22-STALE", SourceType: "manual", TraceabilityRequired: true, Lines: []LineInput{{ProductID: f.product, DepartmentID: f.department, RequiredQuantity: 2}}, IdempotencyKey: "p22-stale-create"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.LinkTraceBox(ctx, f.principal, item.ID, TraceLinkInput{LineID: item.Lines[0].ID, TraceBoxIdentifier: box.OpaqueIdentifier, Quantity: 2, ExpectedVersion: item.Version, IdempotencyKey: "p22-stale-link"})
	if err != nil {
		t.Fatal(err)
	}
	allocationID := item.Lines[0].TraceLinks[0].AllocationEventID
	item, _, err = f.service.UnlinkTraceBox(ctx, f.principal, item.ID, allocationID, TraceUnlinkInput{ExpectedVersion: item.Version, IdempotencyKey: "p22-unlink"})
	if err != nil || item.Lines[0].TracedQuantity != 0 {
		t.Fatalf("unlink=%#v err=%v", item.Lines[0], err)
	}
	item, _, err = f.service.LinkTraceBox(ctx, f.principal, item.ID, TraceLinkInput{LineID: item.Lines[0].ID, TraceBoxIdentifier: box.OpaqueIdentifier, Quantity: 2, ExpectedVersion: item.Version, IdempotencyKey: "p22-relink"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.inventory.StockIn(ctx, f.principal, inventory.CommandInput{ProductID: f.product, Quantity: 2, Reason: "Stale trace test", IdempotencyKey: "p22-stale-stock"}); err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.Allocate(ctx, f.principal, item.ID, ActionInput{ExpectedVersion: item.Version, IdempotencyKey: "p22-stale-allocate"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.Transition(ctx, f.principal, item.ID, TransitionInput{TargetStatus: "picking", ExpectedVersion: item.Version, IdempotencyKey: "p22-stale-picking"})
	if err != nil {
		t.Fatal(err)
	}
	traceService := traceability.NewService(f.db, authorization.NewService(f.db))
	if _, _, err = traceService.AddContent(ctx, f.principal, box.ID, traceability.ContentInput{ProductID: f.product, Quantity: 1, IdempotencyKey: "p22-invalidate"}); err != nil {
		t.Fatal(err)
	}
	line := item.Lines[0]
	if _, _, err = f.service.UpdateProgress(ctx, f.principal, item.ID, line.ID, ProgressInput{ReadyQuantity: 2, ExpectedVersion: line.Version, IdempotencyKey: "p22-stale-progress"}); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("stale progress=%v", err)
	}
}

func readyTraceBox(t *testing.T, f *fixture, contents []traceability.ContentInput, key string) traceability.TraceBox {
	t.Helper()
	ctx := context.Background()
	service := traceability.NewService(f.db, authorization.NewService(f.db))
	box, _, err := service.Create(ctx, f.principal, traceability.CreateInput{IdempotencyKey: key + "-create"})
	if err != nil {
		t.Fatal(err)
	}
	lines := []traceability.QCLine{}
	for index, input := range contents {
		input.IdempotencyKey = fmt.Sprintf("%s-content-%d", key, index)
		box, _, err = service.AddContent(ctx, f.principal, box.ID, input)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, traceability.QCLine{ProductID: input.ProductID, CheckedQuantity: input.Quantity, PassedQuantity: input.Quantity})
	}
	box, _, err = service.RecordQC(ctx, f.principal, box.ID, traceability.QCInput{Lines: lines, IdempotencyKey: key + "-qc"})
	if err != nil {
		t.Fatal(err)
	}
	box, _, err = service.CompletePacking(ctx, f.principal, box.ID, traceability.GateInput{IdempotencyKey: key + "-packing"})
	if err != nil {
		t.Fatal(err)
	}
	passed := true
	box, _, err = service.CompleteFinalCheck(ctx, f.principal, box.ID, traceability.GateInput{Passed: &passed, IdempotencyKey: key + "-final"})
	if err != nil {
		t.Fatal(err)
	}
	box, _, err = service.MarkReady(ctx, f.principal, box.ID, traceability.GateInput{IdempotencyKey: key + "-ready"})
	if err != nil {
		t.Fatal(err)
	}
	return box
}

func currentLine(item Consignment, id string) Line {
	for _, line := range item.Lines {
		if line.ID == id {
			return line
		}
	}
	return Line{}
}
