// This file verifies tenant isolation, idempotency, immutable history, custody, and inventory neutrality against PostgreSQL.
package traceability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/platform/auth"
	"github.com/commerceops/commerceops/services/api/internal/platform/authorization"
	"github.com/jackc/pgx/v5/pgxpool"
)

type testFixture struct {
	db                                                         *pgxpool.Pool
	service                                                    *Service
	manager, viewer, other                                     auth.Principal
	product, employee, department, otherProduct, otherEmployee string
}

func setupTraceability(t *testing.T) *testFixture {
	t.Helper()
	connection := os.Getenv("TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	base, err := pgxpool.New(ctx, connection)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("traceability_test_%d", time.Now().UnixNano())
	if _, err = base.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		base.Close()
		t.Fatal(err)
	}
	parsed, err := url.Parse(connection)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, err := pgxpool.New(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := filepath.Glob("../../../migrations/*.up.sql")
	if err != nil || len(migrations) == 0 {
		t.Fatal("migration files missing", err)
	}
	for _, path := range migrations {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = db.Exec(ctx, string(data)); err != nil {
			t.Fatalf("apply %s: %v", path, err)
		}
	}
	t.Cleanup(func() {
		db.Close()
		if _, dropErr := base.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); dropErr != nil {
			t.Error(dropErr)
		}
		base.Close()
	})

	suffix := fmt.Sprint(time.Now().UnixNano())
	var company, otherCompany, managerUser, viewerUser, otherUser, managerRole, viewerRole, otherRole string
	mustScan(t, db, `INSERT INTO companies(name) VALUES($1) RETURNING id`, []any{"Traceability A " + suffix}, &company)
	mustScan(t, db, `INSERT INTO companies(name) VALUES($1) RETURNING id`, []any{"Traceability B " + suffix}, &otherCompany)
	mustScan(t, db, `INSERT INTO users(email,password_hash) VALUES($1,'test') RETURNING id`, []any{"trace-manager-" + suffix + "@example.test"}, &managerUser)
	mustScan(t, db, `INSERT INTO users(email,password_hash) VALUES($1,'test') RETURNING id`, []any{"trace-viewer-" + suffix + "@example.test"}, &viewerUser)
	mustScan(t, db, `INSERT INTO users(email,password_hash) VALUES($1,'test') RETURNING id`, []any{"trace-other-" + suffix + "@example.test"}, &otherUser)
	mustExec(t, db, `INSERT INTO company_users(company_id,user_id) VALUES($1,$2),($1,$3),($4,$5)`, company, managerUser, viewerUser, otherCompany, otherUser)
	mustScan(t, db, `INSERT INTO roles(company_id,name) VALUES($1,'Trace Manager') RETURNING id`, []any{company}, &managerRole)
	mustScan(t, db, `INSERT INTO roles(company_id,name) VALUES($1,'Trace Viewer') RETURNING id`, []any{company}, &viewerRole)
	mustScan(t, db, `INSERT INTO roles(company_id,name) VALUES($1,'Other Trace Manager') RETURNING id`, []any{otherCompany}, &otherRole)
	mustExec(t, db, `INSERT INTO role_permissions(company_id,role_id,permission_key) VALUES($1,$2,'traceability.view'),($1,$2,'traceability.manage'),($1,$3,'traceability.view'),($4,$5,'traceability.view'),($4,$5,'traceability.manage')`, company, managerRole, viewerRole, otherCompany, otherRole)
	mustExec(t, db, `INSERT INTO company_user_roles(company_id,user_id,role_id) VALUES($1,$2,$3),($1,$4,$5),($6,$7,$8)`, company, managerUser, managerRole, viewerUser, viewerRole, otherCompany, otherUser, otherRole)
	mustExec(t, db, `INSERT INTO module_entitlements(company_id,module_key,enabled) VALUES($1,'traceability',true),($2,'traceability',true)`, company, otherCompany)

	f := &testFixture{db: db, service: NewService(db, authorization.NewService(db)), manager: auth.Principal{CompanyID: company, UserID: managerUser}, viewer: auth.Principal{CompanyID: company, UserID: viewerUser}, other: auth.Principal{CompanyID: otherCompany, UserID: otherUser}}
	mustScan(t, db, `INSERT INTO products(company_id,internal_code,name) VALUES($1,'TRACE-1','Trace Product') RETURNING id`, []any{company}, &f.product)
	mustScan(t, db, `INSERT INTO employees(company_id,user_id,display_name) VALUES($1,$2,'Trace Manager') RETURNING id`, []any{company, managerUser}, &f.employee)
	mustScan(t, db, `INSERT INTO consignment_departments(company_id,name,created_by) VALUES($1,'Trace Department',$2) RETURNING id`, []any{company, managerUser}, &f.department)
	mustScan(t, db, `INSERT INTO products(company_id,internal_code,name) VALUES($1,'OTHER-TRACE','Other Trace Product') RETURNING id`, []any{otherCompany}, &f.otherProduct)
	mustScan(t, db, `INSERT INTO employees(company_id,user_id,display_name) VALUES($1,$2,'Other Employee') RETURNING id`, []any{otherCompany, otherUser}, &f.otherEmployee)
	return f
}

func TestTraceabilityFoundation(t *testing.T) {
	f := setupTraceability(t)
	ctx := context.Background()
	created, replayed, err := f.service.Create(ctx, f.manager, CreateInput{Label: textPointer("Returns staging"), IdempotencyKey: "trace-create"})
	if err != nil || replayed || !regexp.MustCompile(`^TBX_[A-Z2-7]{26}$`).MatchString(created.OpaqueIdentifier) || len(created.Events) != 1 {
		t.Fatalf("create=%#v replay=%v err=%v", created, replayed, err)
	}
	repeated, replayed, err := f.service.Create(ctx, f.manager, CreateInput{Label: textPointer("Returns staging"), IdempotencyKey: "trace-create"})
	if err != nil || !replayed || repeated.ID != created.ID || repeated.OpaqueIdentifier != created.OpaqueIdentifier {
		t.Fatalf("replay=%#v replayed=%v err=%v", repeated, replayed, err)
	}
	if _, _, err = f.service.Create(ctx, f.manager, CreateInput{Label: textPointer("Changed"), IdempotencyKey: "trace-create"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting replay=%v", err)
	}

	item, replayed, err := f.service.AddContent(ctx, f.manager, created.ID, ContentInput{ProductID: f.product, Quantity: 5, Notes: textPointer("counted"), IdempotencyKey: "trace-add"})
	if err != nil || replayed || len(item.Contents) != 1 || item.Contents[0].Quantity != 5 {
		t.Fatalf("add=%#v replay=%v err=%v", item, replayed, err)
	}
	item, replayed, err = f.service.AddContent(ctx, f.manager, created.ID, ContentInput{ProductID: f.product, Quantity: 5, Notes: textPointer("counted"), IdempotencyKey: "trace-add"})
	if err != nil || !replayed || item.Contents[0].Quantity != 5 {
		t.Fatalf("add replay=%#v replay=%v err=%v", item, replayed, err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	for _, key := range []string{"trace-remove-a", "trace-remove-b"} {
		go func(idempotencyKey string) {
			<-start
			_, _, removeErr := f.service.RemoveContent(ctx, f.manager, created.ID, ContentInput{ProductID: f.product, Quantity: 4, IdempotencyKey: idempotencyKey})
			results <- removeErr
		}(key)
	}
	close(start)
	var successful, insufficient int
	for range 2 {
		switch removeErr := <-results; {
		case removeErr == nil:
			successful++
		case errors.Is(removeErr, ErrQuantity):
			insufficient++
		default:
			t.Fatalf("concurrent removal=%v", removeErr)
		}
	}
	if successful != 1 || insufficient != 1 {
		t.Fatalf("successful=%d insufficient=%d", successful, insufficient)
	}
	item, err = f.service.Get(ctx, f.viewer, created.ID)
	if err != nil || item.Contents[0].Quantity != 1 {
		t.Fatalf("viewer get=%#v err=%v", item, err)
	}
	if _, _, err = f.service.AddContent(ctx, f.viewer, created.ID, ContentInput{ProductID: f.product, Quantity: 1, IdempotencyKey: "viewer-denied"}); !errors.Is(err, authorization.ErrPermissionDenied) {
		t.Fatalf("viewer mutation=%v", err)
	}
	if _, _, err = f.service.AddContent(ctx, f.manager, created.ID, ContentInput{ProductID: f.otherProduct, Quantity: 1, IdempotencyKey: "cross-product"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-company product=%v", err)
	}

	item, _, err = f.service.TransferCustody(ctx, f.manager, created.ID, CustodyInput{EmployeeID: &f.employee, Notes: textPointer("received"), IdempotencyKey: "custody-employee"})
	if err != nil || item.CurrentCustody == nil || item.CurrentCustody.EmployeeID == nil || *item.CurrentCustody.EmployeeID != f.employee {
		t.Fatalf("employee custody=%#v err=%v", item.CurrentCustody, err)
	}
	item, _, err = f.service.TransferCustody(ctx, f.manager, created.ID, CustodyInput{DepartmentID: &f.department, IdempotencyKey: "custody-department"})
	if err != nil || item.CurrentCustody == nil || item.CurrentCustody.DepartmentID == nil || *item.CurrentCustody.DepartmentID != f.department {
		t.Fatalf("department custody=%#v err=%v", item.CurrentCustody, err)
	}
	if _, _, err = f.service.TransferCustody(ctx, f.manager, created.ID, CustodyInput{EmployeeID: &f.otherEmployee, IdempotencyKey: "cross-custody"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-company custody=%v", err)
	}
	options, err := f.service.Options(ctx, f.viewer)
	if err != nil || len(options.Products) != 1 || options.Products[0].ID != f.product || len(options.Employees) != 1 || options.Employees[0].ID != f.employee || len(options.Departments) != 1 || options.Departments[0].ID != f.department {
		t.Fatalf("company options=%#v err=%v", options, err)
	}
	resolved, err := f.service.Resolve(ctx, f.manager, "  "+created.OpaqueIdentifier+"  ")
	if err != nil || resolved.ID != created.ID {
		t.Fatalf("resolve=%#v err=%v", resolved, err)
	}
	if _, err = f.service.Resolve(ctx, f.other, created.OpaqueIdentifier); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-company resolve=%v", err)
	}
	if _, err = f.db.Exec(ctx, `UPDATE trace_box_events SET notes='rewrite' WHERE company_id=$1 AND trace_box_id=$2`, f.manager.CompanyID, created.ID); err == nil {
		t.Fatal("immutable trace event was updated")
	}
	if _, err = f.db.Exec(ctx, `UPDATE trace_box_content_changes SET quantity_delta=2 WHERE company_id=$1 AND trace_box_id=$2`, f.manager.CompanyID, created.ID); err == nil {
		t.Fatal("immutable content history was updated")
	}
	if _, err = f.db.Exec(ctx, `DELETE FROM trace_box_custody_changes WHERE company_id=$1 AND trace_box_id=$2`, f.manager.CompanyID, created.ID); err == nil {
		t.Fatal("immutable custody history was deleted")
	}
	var inventoryRows, audits int
	mustScan(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, []any{f.manager.CompanyID}, &inventoryRows)
	mustScan(t, f.db, `SELECT count(*) FROM audit_logs WHERE company_id=$1 AND target_type='trace_box'`, []any{f.manager.CompanyID}, &audits)
	if inventoryRows != 0 || audits != 5 {
		t.Fatalf("inventory rows=%d audits=%d", inventoryRows, audits)
	}
	mustExec(t, f.db, `UPDATE module_entitlements SET enabled=false WHERE company_id=$1 AND module_key='traceability'`, f.manager.CompanyID)
	if _, err = f.service.Get(ctx, f.manager, created.ID); !errors.Is(err, authorization.ErrModuleUnavailable) {
		t.Fatalf("disabled entitlement=%v", err)
	}
}

func TestTraceabilityWorkerWorkflow(t *testing.T) {
	f := setupTraceability(t)
	ctx := context.Background()
	mustExec(t, f.db, `INSERT INTO consignment_department_members(company_id,department_id,employee_id,assigned_by) VALUES($1,$2,$3,$4)`, f.manager.CompanyID, f.department, f.employee, f.manager.UserID)
	box, _, err := f.service.Create(ctx, f.manager, CreateInput{Label: textPointer("QC lane"), IdempotencyKey: "workflow-create"})
	if err != nil {
		t.Fatal(err)
	}
	box, _, err = f.service.AddContent(ctx, f.manager, box.ID, ContentInput{ProductID: f.product, Quantity: 5, IdempotencyKey: "workflow-content"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.service.CompletePacking(ctx, f.manager, box.ID, GateInput{IdempotencyKey: "packing-too-soon"}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("packing before QC=%v", err)
	}

	reason, work := "wrong_sticker", "sticker_replacement"
	qcInput := QCInput{Lines: []QCLine{{ProductID: f.product, CheckedQuantity: 5, PassedQuantity: 3, RejectedQuantity: 2, RejectionReason: &reason, RequiredWork: &work}}, IdempotencyKey: "workflow-qc-reject"}
	box, replayed, err := f.service.RecordQC(ctx, f.manager, box.ID, qcInput)
	if err != nil || replayed || box.Workflow.Status != "rework_required" || len(box.Workflow.WorkRequirements) != 1 {
		t.Fatalf("rejected QC=%#v replay=%v err=%v", box.Workflow, replayed, err)
	}
	box, replayed, err = f.service.RecordQC(ctx, f.manager, box.ID, qcInput)
	if err != nil || !replayed || len(box.Workflow.WorkRequirements) != 1 {
		t.Fatalf("QC replay=%#v replay=%v err=%v", box.Workflow, replayed, err)
	}
	if _, _, err = f.service.RecordQC(ctx, f.manager, box.ID, QCInput{Lines: []QCLine{{ProductID: f.product, CheckedQuantity: 4, PassedQuantity: 4}}, IdempotencyKey: "workflow-qc-partial"}); !errors.Is(err, ErrQuantity) {
		t.Fatalf("partial QC=%v", err)
	}

	requirementID := box.Workflow.WorkRequirements[0].ID
	box, replayed, err = f.service.CompleteWork(ctx, f.manager, box.ID, requirementID, CompleteWorkInput{IdempotencyKey: "workflow-rework"})
	if err != nil || replayed || box.Workflow.Status != "awaiting_recheck" {
		t.Fatalf("rework=%#v replay=%v err=%v", box.Workflow, replayed, err)
	}
	if _, _, err = f.service.CompletePacking(ctx, f.manager, box.ID, GateInput{IdempotencyKey: "packing-before-recheck"}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("packing before recheck=%v", err)
	}
	box, _, err = f.service.RecordQC(ctx, f.manager, box.ID, QCInput{Lines: []QCLine{{ProductID: f.product, CheckedQuantity: 5, PassedQuantity: 5}}, IdempotencyKey: "workflow-qc-pass"})
	if err != nil || box.Workflow.Status != "qc_passed" {
		t.Fatalf("passing QC=%#v err=%v", box.Workflow, err)
	}
	box, _, err = f.service.CompletePacking(ctx, f.manager, box.ID, GateInput{IdempotencyKey: "workflow-packing"})
	if err != nil || box.Workflow.Status != "packed" {
		t.Fatalf("packing=%#v err=%v", box.Workflow, err)
	}
	failed := false
	box, _, err = f.service.CompleteFinalCheck(ctx, f.manager, box.ID, GateInput{Passed: &failed, Notes: textPointer("seal needs replacement"), IdempotencyKey: "workflow-final-fail"})
	if err != nil || box.Workflow.Status != "final_check_failed" {
		t.Fatalf("failed final=%#v err=%v", box.Workflow, err)
	}
	if _, _, err = f.service.MarkReady(ctx, f.manager, box.ID, GateInput{IdempotencyKey: "ready-after-fail"}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("ready after failed final=%v", err)
	}
	box, _, err = f.service.CompletePacking(ctx, f.manager, box.ID, GateInput{IdempotencyKey: "workflow-repacking"})
	if err != nil {
		t.Fatal(err)
	}
	passed := true
	box, _, err = f.service.CompleteFinalCheck(ctx, f.manager, box.ID, GateInput{Passed: &passed, IdempotencyKey: "workflow-final-pass"})
	if err != nil || box.Workflow.Status != "final_check_passed" {
		t.Fatalf("passed final=%#v err=%v", box.Workflow, err)
	}
	box, _, err = f.service.MarkReady(ctx, f.manager, box.ID, GateInput{IdempotencyKey: "workflow-ready"})
	if err != nil || box.Workflow.Status != "ready_for_shipment" || box.Workflow.ReadyAt == nil {
		t.Fatalf("ready=%#v err=%v", box.Workflow, err)
	}

	box, _, err = f.service.SendHandover(ctx, f.manager, box.ID, HandoverInput{DepartmentID: &f.department, IdempotencyKey: "workflow-handover"})
	if err != nil || box.Workflow.Status != "in_transit" || box.Workflow.PendingHandover == nil {
		t.Fatalf("handover=%#v err=%v", box.Workflow, err)
	}
	if _, _, err = f.service.AddContent(ctx, f.manager, box.ID, ContentInput{ProductID: f.product, Quantity: 1, IdempotencyKey: "content-in-transit"}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("content in transit=%v", err)
	}
	handoverID := box.Workflow.PendingHandover.ID
	box, replayed, err = f.service.ReceiveHandover(ctx, f.manager, box.ID, handoverID, ReceiveHandoverInput{IdempotencyKey: "workflow-receive"})
	if err != nil || replayed || box.Workflow.PendingHandover != nil || box.CurrentCustody == nil || box.CurrentCustody.EmployeeID == nil || *box.CurrentCustody.EmployeeID != f.employee {
		t.Fatalf("receive box=%#v custody=%#v replay=%v err=%v", box.Workflow, box.CurrentCustody, replayed, err)
	}
	box, replayed, err = f.service.ReceiveHandover(ctx, f.manager, box.ID, handoverID, ReceiveHandoverInput{IdempotencyKey: "workflow-receive"})
	if err != nil || !replayed {
		t.Fatalf("receive replay=%v err=%v", replayed, err)
	}

	var inventoryRows int
	mustScan(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, []any{f.manager.CompanyID}, &inventoryRows)
	if inventoryRows != 0 {
		t.Fatalf("workflow changed inventory: %d rows", inventoryRows)
	}
	if _, err = f.db.Exec(ctx, `DELETE FROM trace_box_work_requirements WHERE company_id=$1 AND trace_box_id=$2`, f.manager.CompanyID, box.ID); err == nil {
		t.Fatal("work history was deleted")
	}
}

func TestConcurrentHandoverAllowsOnePendingTransfer(t *testing.T) {
	f := setupTraceability(t)
	ctx := context.Background()
	box, _, err := f.service.Create(ctx, f.manager, CreateInput{IdempotencyKey: "handover-race-create"})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, key := range []string{"handover-race-a", "handover-race-b"} {
		go func(idempotencyKey string) {
			<-start
			_, _, sendErr := f.service.SendHandover(ctx, f.manager, box.ID, HandoverInput{EmployeeID: &f.employee, IdempotencyKey: idempotencyKey})
			results <- sendErr
		}(key)
	}
	close(start)
	var successful, rejected int
	for range 2 {
		switch sendErr := <-results; {
		case sendErr == nil:
			successful++
		case errors.Is(sendErr, ErrInvalidTransition):
			rejected++
		default:
			t.Fatalf("concurrent handover=%v", sendErr)
		}
	}
	if successful != 1 || rejected != 1 {
		t.Fatalf("successful=%d rejected=%d", successful, rejected)
	}
}

func TestTraceabilityMigrationRoundTrip(t *testing.T) {
	f := setupTraceability(t)
	phase23Down, err := os.ReadFile("../../../migrations/000030_operations_analytics_indexes.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(context.Background(), string(phase23Down)); err != nil {
		t.Fatal(err)
	}
	phase22Down, err := os.ReadFile("../../../migrations/000029_consignment_traceability_integration.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(context.Background(), string(phase22Down)); err != nil {
		t.Fatal(err)
	}
	phase21Down, err := os.ReadFile("../../../migrations/000028_traceability_worker_workflows.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(context.Background(), string(phase21Down)); err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile("../../../migrations/000027_traceability_foundation.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(context.Background(), string(down)); err != nil {
		t.Fatal(err)
	}
	up, err := os.ReadFile("../../../migrations/000027_traceability_foundation.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(context.Background(), string(up)); err != nil {
		t.Fatal(err)
	}
	phase21Up, err := os.ReadFile("../../../migrations/000028_traceability_worker_workflows.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(context.Background(), string(phase21Up)); err != nil {
		t.Fatal(err)
	}
	phase22Up, err := os.ReadFile("../../../migrations/000029_consignment_traceability_integration.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(context.Background(), string(phase22Up)); err != nil {
		t.Fatal(err)
	}
	phase23Up, err := os.ReadFile("../../../migrations/000030_operations_analytics_indexes.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(context.Background(), string(phase23Up)); err != nil {
		t.Fatal(err)
	}
	var table string
	if err = f.db.QueryRow(context.Background(), `SELECT 'trace_boxes'::regclass::text`).Scan(&table); err != nil || table != "trace_boxes" {
		t.Fatalf("table=%q err=%v", table, err)
	}
	var index string
	if err = f.db.QueryRow(context.Background(), `SELECT 'trace_box_events_company_time_idx'::regclass::text`).Scan(&index); err != nil || index != "trace_box_events_company_time_idx" {
		t.Fatalf("index=%q err=%v", index, err)
	}
}

func TestTraceabilityHTTPBoundary(t *testing.T) {
	f := setupTraceability(t)
	handler := NewHTTPHandler(f.service)
	body, err := json.Marshal(CreateInput{Label: textPointer("HTTP Box"), IdempotencyKey: "http-create"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/trace-boxes", bytes.NewReader(body))
	request = request.WithContext(auth.WithPrincipal(request.Context(), f.manager))
	response := httptest.NewRecorder()
	handler.Boxes(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}
	var result struct {
		TraceBox TraceBox `json:"trace_box"`
	}
	if err = json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/trace-box-resolutions/"+result.TraceBox.OpaqueIdentifier, nil)
	request.SetPathValue("opaque_identifier", result.TraceBox.OpaqueIdentifier)
	request = request.WithContext(auth.WithPrincipal(request.Context(), f.manager))
	response = httptest.NewRecorder()
	handler.Resolve(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(result.TraceBox.ID)) {
		t.Fatalf("resolve status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodDelete, "/api/v1/trace-boxes", nil)
	request = request.WithContext(auth.WithPrincipal(request.Context(), f.manager))
	response = httptest.NewRecorder()
	handler.Boxes(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method status=%d", response.Code)
	}
}

func mustScan(t *testing.T, db *pgxpool.Pool, query string, args []any, destinations ...any) {
	t.Helper()
	if err := db.QueryRow(context.Background(), query, args...).Scan(destinations...); err != nil {
		t.Fatal(err)
	}
}

func mustExec(t *testing.T, db *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(context.Background(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func textPointer(value string) *string { return &value }
