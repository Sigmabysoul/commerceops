package batch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/platform/objectstorage"
	"github.com/commerceops/commerceops/services/api/internal/platform/pdfgenerator"
	"github.com/jackc/pgx/v5/pgxpool"
)

type batchFixture struct {
	db                               *pgxpool.Pool
	service                          *Service
	companyA, companyB, userID       string
	roleA, productID                 string
	defaultWorkerID, productWorkerID string
	principalA, principalB           auth.Principal
	storage                          *objectstorage.Local
	generator                        *recordingGenerator
}

type recordingGenerator struct {
	calls [][]pdfgenerator.Page
	err   error
}

func (g *recordingGenerator) Generate(_ context.Context, pages []pdfgenerator.Page, invoices bool) (pdfgenerator.Result, error) {
	g.calls = append(g.calls, append([]pdfgenerator.Page(nil), pages...))
	if g.err != nil {
		return pdfgenerator.Result{}, g.err
	}
	result := pdfgenerator.Result{Labels: []byte("%PDF-sanitized-labels")}
	if invoices {
		result.Invoices = []byte("%PDF-sanitized-invoices")
	}
	return result, nil
}

func setupBatch(t *testing.T) *batchFixture {
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
	f := &batchFixture{db: db}
	suffix := fmt.Sprint(time.Now().UnixNano())
	scanBatchTest(t, db, `INSERT INTO companies(name) VALUES($1) RETURNING id`, []any{"P4 A " + suffix}, &f.companyA)
	scanBatchTest(t, db, `INSERT INTO companies(name) VALUES($1) RETURNING id`, []any{"P4 B " + suffix}, &f.companyB)
	scanBatchTest(t, db, `INSERT INTO users(email,password_hash) VALUES($1,'test') RETURNING id`, []any{"p4-" + suffix + "@example.test"}, &f.userID)
	execBatchTest(t, db, `INSERT INTO company_users(company_id,user_id) VALUES($1,$3),($2,$3)`, f.companyA, f.companyB, f.userID)
	for _, company := range []string{f.companyA, f.companyB} {
		var role string
		scanBatchTest(t, db, `INSERT INTO roles(company_id,name) VALUES($1,'Batch Operator') RETURNING id`, []any{company}, &role)
		execBatchTest(t, db, `INSERT INTO role_permissions(company_id,role_id,permission_key) VALUES($1,$2,'labels.process'),($1,$2,'labels.print'),($1,$2,'labels.reprint'),($1,$2,'employees.manage')`, company, role)
		execBatchTest(t, db, `INSERT INTO company_user_roles(company_id,user_id,role_id) VALUES($1,$2,$3)`, company, f.userID, role)
		execBatchTest(t, db, `INSERT INTO module_entitlements(company_id,module_key,enabled) VALUES($1,'flipkart',true),($1,'amazon',true),($1,'meesho',true),($1,'myntra',true),($1,'snapdeal',true)`, company)
		if company == f.companyA {
			f.roleA = role
		}
	}
	scanBatchTest(t, db, `INSERT INTO products(company_id,internal_code,name) VALUES($1,'P4-PRODUCT','Phase 4 Product') RETURNING id`, []any{f.companyA}, &f.productID)
	scanBatchTest(t, db, `INSERT INTO employees(company_id,display_name) VALUES($1,'Default Worker') RETURNING id`, []any{f.companyA}, &f.defaultWorkerID)
	scanBatchTest(t, db, `INSERT INTO employees(company_id,display_name) VALUES($1,'Product Worker') RETURNING id`, []any{f.companyA}, &f.productWorkerID)
	execBatchTest(t, db, `INSERT INTO worker_assignment_rules(company_id,marketplace_key,product_id,employee_id,priority) VALUES($1,'flipkart',NULL,$2,100),($1,'flipkart',$3,$4,10)`, f.companyA, f.defaultWorkerID, f.productID, f.productWorkerID)
	execBatchTest(t, db, `INSERT INTO worker_assignment_rules(company_id,marketplace_key,product_id,employee_id,priority) VALUES($1,'amazon',NULL,$2,100),($1,'amazon',$3,$4,10)`, f.companyA, f.defaultWorkerID, f.productID, f.productWorkerID)
	execBatchTest(t, db, `INSERT INTO worker_assignment_rules(company_id,marketplace_key,product_id,employee_id,priority) VALUES($1,'meesho',NULL,$2,100),($1,'meesho',$3,$4,10)`, f.companyA, f.defaultWorkerID, f.productID, f.productWorkerID)
	execBatchTest(t, db, `INSERT INTO worker_assignment_rules(company_id,marketplace_key,product_id,employee_id,priority) VALUES($1,'snapdeal',NULL,$2,100),($1,'snapdeal',$3,$4,10)`, f.companyA, f.defaultWorkerID, f.productID, f.productWorkerID)
	f.storage, err = objectstorage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f.generator = &recordingGenerator{}
	f.service = NewPrintingService(db, authorization.NewService(db), f.storage, f.generator).
		RegisterPrintGenerator("amazon", "amazon-a4-enriched-v1", f.generator).
		RegisterPrintGenerator("meesho", pdfgenerator.SourcePageGenerationVersion, f.generator).
		RegisterPrintGenerator("snapdeal", "snapdeal-packslip-enriched-v1", f.generator)
	f.principalA = auth.Principal{CompanyID: f.companyA, UserID: f.userID}
	f.principalB = auth.Principal{CompanyID: f.companyB, UserID: f.userID}
	t.Cleanup(func() { cleanupBatch(t, f); db.Close() })
	return f
}

func TestMyntraMissingQuantityCannotBecomeReady(t *testing.T) {
	f := setupBatch(t)
	ctx := context.Background()
	seed := fmt.Sprintf("myntra-%s-%d", f.companyA, time.Now().UnixNano())
	hash := sha256.Sum256([]byte(seed))
	var sourceID, jobID, orderID string
	scanBatchTest(t, f.db, `INSERT INTO source_files(company_id,marketplace_key,storage_key,original_filename,content_type,size_bytes,sha256,uploaded_by) VALUES($1,'myntra',$2,'orders.csv','text/csv',1,$3,$4) RETURNING id`, []any{f.companyA, seed, hex.EncodeToString(hash[:]), f.userID}, &sourceID)
	scanBatchTest(t, f.db, `INSERT INTO processing_jobs(company_id,source_file_id,marketplace_key,status,parser_version,total_pages,processed_pages) VALUES($1,$2,'myntra','needs_review','myntra-packed-orders-csv-v1',1,1) RETURNING id`, []any{f.companyA, sourceID}, &jobID)
	scanBatchTest(t, f.db, `INSERT INTO marketplace_orders(company_id,marketplace_key,source_file_id,processing_job_id,source_page,marketplace_order_id,awb,status,parser_version) VALUES($1,'myntra',$2,$3,2,'7000000099','MYSP1000000099','needs_review','myntra-packed-orders-csv-v1') RETURNING id`, []any{f.companyA, sourceID, jobID}, &orderID)
	execBatchTest(t, f.db, `INSERT INTO marketplace_order_items(company_id,order_id,raw_sku,product_id,quantity,quantity_source,resolution_status) VALUES($1,$2,'SANITIZED-SKU',$3,NULL,'missing','resolved')`, f.companyA, orderID, f.productID)
	eligible, err := f.service.EligibleOrders(ctx, f.principalA, "myntra")
	if err != nil || len(eligible) != 1 || eligible[0].UnresolvedCount == 0 {
		t.Fatalf("eligible=%#v err=%v", eligible, err)
	}
	created, _, err := f.service.Create(ctx, f.principalA, CreateInput{MarketplaceKey: "myntra", OrderIDs: []string{orderID}, IdempotencyKey: "myntra-missing-quantity"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Ready(ctx, f.principalA, created.ID); !errors.Is(err, ErrUnresolved) {
		t.Fatalf("ready error=%v", err)
	}
	var events int
	scanBatchTest(t, f.db, `SELECT count(*) FROM inventory_outbound_events WHERE company_id=$1 AND batch_id=$2`, []any{f.companyA, created.ID}, &events)
	if events != 0 {
		t.Fatalf("outbound events=%d", events)
	}
}

func (f *batchFixture) amazonOrder(t *testing.T, quantity int) string {
	t.Helper()
	seed := fmt.Sprintf("amazon-%s-%d", f.companyA, time.Now().UnixNano())
	hash := sha256.Sum256([]byte(seed))
	var sourceID, jobID, orderID string
	scanBatchTest(t, f.db, `INSERT INTO source_files(company_id,marketplace_key,storage_key,original_filename,content_type,size_bytes,sha256,uploaded_by) VALUES($1,'amazon',$2,'amazon.pdf','application/pdf',1,$3,$4) RETURNING id`, []any{f.companyA, seed, hex.EncodeToString(hash[:]), f.userID}, &sourceID)
	if err := f.storage.Put(context.Background(), seed, bytes.NewReader([]byte("%PDF-amazon-source")), 18, "application/pdf"); err != nil {
		t.Fatal(err)
	}
	scanBatchTest(t, f.db, `INSERT INTO processing_jobs(company_id,source_file_id,marketplace_key,status,parser_version,total_pages,processed_pages) VALUES($1,$2,'amazon','processed','amazon-associated-v3',2,2) RETURNING id`, []any{f.companyA, sourceID}, &jobID)
	scanBatchTest(t, f.db, `INSERT INTO marketplace_orders(company_id,marketplace_key,source_file_id,processing_job_id,source_page,marketplace_order_id,awb,status,parser_version) VALUES($1,'amazon',$2,$3,1,'406-9090909-8080808','TRACKAMAZON1','resolved','amazon-associated-v3') RETURNING id`, []any{f.companyA, sourceID, jobID}, &orderID)
	execBatchTest(t, f.db, `INSERT INTO marketplace_order_documents(company_id,order_id,source_file_id,source_page,document_role,extraction_method) VALUES($1,$2,$3,1,'shipping_label','ocr'),($1,$2,$3,2,'invoice','text')`, f.companyA, orderID, sourceID)
	execBatchTest(t, f.db, `INSERT INTO marketplace_order_items(company_id,order_id,raw_sku,product_id,quantity,quantity_source,resolution_status) VALUES($1,$2,'AMAZON-SKU',$3,$4,'extracted','resolved')`, f.companyA, orderID, f.productID, quantity)
	return orderID
}

func (f *batchFixture) meeshoOrder(t *testing.T, quantity int) string {
	t.Helper()
	seed := fmt.Sprintf("meesho-%s-%d", f.companyA, time.Now().UnixNano())
	hash := sha256.Sum256([]byte(seed))
	var sourceID, jobID, orderID string
	scanBatchTest(t, f.db, `INSERT INTO source_files(company_id,marketplace_key,storage_key,original_filename,content_type,size_bytes,sha256,uploaded_by) VALUES($1,'meesho',$2,'meesho.pdf','application/pdf',1,$3,$4) RETURNING id`, []any{f.companyA, seed, hex.EncodeToString(hash[:]), f.userID}, &sourceID)
	source := []byte("%PDF-meesho-source")
	if err := f.storage.Put(context.Background(), seed, bytes.NewReader(source), int64(len(source)), "application/pdf"); err != nil {
		t.Fatal(err)
	}
	scanBatchTest(t, f.db, `INSERT INTO processing_jobs(company_id,source_file_id,marketplace_key,status,parser_version,total_pages,processed_pages) VALUES($1,$2,'meesho','processed','meesho-labeled-v1',1,1) RETURNING id`, []any{f.companyA, sourceID}, &jobID)
	scanBatchTest(t, f.db, `INSERT INTO marketplace_orders(company_id,marketplace_key,source_file_id,processing_job_id,source_page,marketplace_order_id,awb,status,parser_version) VALUES($1,'meesho',$2,$3,1,'100000000009_1','MEESHOAWB90001','resolved','meesho-labeled-v1') RETURNING id`, []any{f.companyA, sourceID, jobID}, &orderID)
	execBatchTest(t, f.db, `INSERT INTO marketplace_order_documents(company_id,order_id,source_file_id,source_page,document_role,extraction_method) VALUES($1,$2,$3,1,'shipping_label','text')`, f.companyA, orderID, sourceID)
	execBatchTest(t, f.db, `INSERT INTO marketplace_order_items(company_id,order_id,raw_sku,product_id,quantity,quantity_source,resolution_status) VALUES($1,$2,'MEESHO-SKU',$3,$4,'extracted','resolved')`, f.companyA, orderID, f.productID, quantity)
	return orderID
}

func (f *batchFixture) order(t *testing.T, company, status string, productID *string, quantity *int) string {
	t.Helper()
	seed := fmt.Sprintf("%s-%d", company, time.Now().UnixNano())
	hash := sha256.Sum256([]byte(seed))
	sha := hex.EncodeToString(hash[:])
	var sourceID, jobID, orderID string
	scanBatchTest(t, f.db, `INSERT INTO source_files(company_id,marketplace_key,storage_key,original_filename,content_type,size_bytes,sha256,uploaded_by) VALUES($1,'flipkart',$2,'batch.pdf','application/pdf',1,$3,$4) RETURNING id`, []any{company, seed, sha, f.userID}, &sourceID)
	if err := f.storage.Put(context.Background(), seed, bytes.NewReader([]byte("%PDF-source")), 11, "application/pdf"); err != nil {
		t.Fatal(err)
	}
	jobStatus := "processed"
	if status == "needs_review" {
		jobStatus = "needs_review"
	}
	scanBatchTest(t, f.db, `INSERT INTO processing_jobs(company_id,source_file_id,marketplace_key,status,parser_version,total_pages,processed_pages) VALUES($1,$2,'flipkart',$3,'fixture',1,1) RETURNING id`, []any{company, sourceID, jobStatus}, &jobID)
	scanBatchTest(t, f.db, `INSERT INTO marketplace_orders(company_id,marketplace_key,source_file_id,processing_job_id,source_page,status,parser_version) VALUES($1,'flipkart',$2,$3,1,$4,'fixture') RETURNING id`, []any{company, sourceID, jobID, status}, &orderID)
	resolution, quantitySource := "unresolved", "missing"
	if productID != nil && quantity != nil {
		resolution, quantitySource = "resolved", "extracted"
	}
	execBatchTest(t, f.db, `INSERT INTO marketplace_order_items(company_id,order_id,raw_sku,product_id,quantity,quantity_source,resolution_status) VALUES($1,$2,'P4-SKU',$3,$4,$5,$6)`, company, orderID, productID, quantity, quantitySource, resolution)
	return orderID
}

func TestBatchFoundationPostgreSQLBehavior(t *testing.T) {
	f := setupBatch(t)
	ctx := context.Background()
	two, three := 2, 3
	first := f.order(t, f.companyA, "resolved", &f.productID, &two)
	second := f.order(t, f.companyA, "resolved", &f.productID, &three)
	unresolved := f.order(t, f.companyA, "needs_review", nil, nil)
	otherTenant := f.order(t, f.companyB, "resolved", nil, nil)

	t.Run("authorization and entitlement precede persistence", func(t *testing.T) {
		execBatchTest(t, f.db, `UPDATE module_entitlements SET enabled=false WHERE company_id=$1 AND module_key='flipkart'`, f.companyA)
		_, _, err := f.service.Create(ctx, f.principalA, CreateInput{MarketplaceKey: "flipkart", OrderIDs: []string{first}, IdempotencyKey: "denied-module"})
		if !errors.Is(err, authorization.ErrModuleUnavailable) {
			t.Fatalf("module error=%v", err)
		}
		execBatchTest(t, f.db, `UPDATE module_entitlements SET enabled=true WHERE company_id=$1 AND module_key='flipkart'`, f.companyA)
		execBatchTest(t, f.db, `DELETE FROM role_permissions WHERE company_id=$1 AND role_id=$2 AND permission_key='labels.process'`, f.companyA, f.roleA)
		_, _, err = f.service.Create(ctx, f.principalA, CreateInput{MarketplaceKey: "flipkart", OrderIDs: []string{first}, IdempotencyKey: "denied-permission"})
		if !errors.Is(err, authorization.ErrPermissionDenied) {
			t.Fatalf("permission error=%v", err)
		}
		execBatchTest(t, f.db, `INSERT INTO role_permissions(company_id,role_id,permission_key) VALUES($1,$2,'labels.process')`, f.companyA, f.roleA)
		var count int
		scanBatchTest(t, f.db, `SELECT count(*) FROM batches WHERE company_id=$1 AND idempotency_key LIKE 'denied-%'`, []any{f.companyA}, &count)
		if count != 0 {
			t.Fatalf("denied requests persisted %d batches", count)
		}
	})

	t.Run("eligible orders are tenant scoped", func(t *testing.T) {
		items, err := f.service.EligibleOrders(ctx, f.principalA, "flipkart")
		if err != nil || len(items) != 3 {
			t.Fatalf("eligible=%#v err=%v", items, err)
		}
		for _, item := range items {
			if item.OrderID == otherTenant {
				t.Fatal("cross-tenant order leaked")
			}
		}
	})

	created, replayed, err := f.service.Create(ctx, f.principalA, CreateInput{MarketplaceKey: "flipkart", OrderIDs: []string{second, first}, IdempotencyKey: "resolved-batch"})
	if err != nil || replayed {
		t.Fatalf("create=%#v replayed=%v err=%v", created, replayed, err)
	}
	if created.OrderCount != 2 || created.UnresolvedCount != 0 || len(created.Members) != 2 || created.Members[0].OrderID != second || created.Members[1].OrderID != first {
		t.Fatalf("created=%#v", created)
	}
	if len(created.ProductTotals) != 1 || created.ProductTotals[0].ProductID != f.productID || created.ProductTotals[0].TotalQuantity != 5 || created.ProductTotals[0].OrderLineCount != 2 {
		t.Fatalf("totals=%#v", created.ProductTotals)
	}

	t.Run("assignment rules require management permission and remain tenant scoped", func(t *testing.T) {
		rules, err := f.service.ListAssignmentRules(ctx, f.principalA, "flipkart")
		if err != nil || len(rules) != 2 {
			t.Fatalf("rules=%#v err=%v", rules, err)
		}
		execBatchTest(t, f.db, `DELETE FROM role_permissions WHERE company_id=$1 AND role_id=$2 AND permission_key='employees.manage'`, f.companyA, f.roleA)
		_, err = f.service.ReplaceAssignmentRules(ctx, f.principalA, ReplaceAssignmentRulesInput{MarketplaceKey: "flipkart", Rules: []AssignmentRuleInput{{EmployeeID: f.defaultWorkerID, Priority: 100}}})
		if !errors.Is(err, authorization.ErrPermissionDenied) {
			t.Fatalf("assignment permission error=%v", err)
		}
		execBatchTest(t, f.db, `INSERT INTO role_permissions(company_id,role_id,permission_key) VALUES($1,$2,'employees.manage')`, f.companyA, f.roleA)
		var otherEmployee string
		scanBatchTest(t, f.db, `INSERT INTO employees(company_id,display_name) VALUES($1,'Other Tenant Worker') RETURNING id`, []any{f.companyB}, &otherEmployee)
		_, err = f.service.ReplaceAssignmentRules(ctx, f.principalA, ReplaceAssignmentRulesInput{MarketplaceKey: "flipkart", Rules: []AssignmentRuleInput{{EmployeeID: otherEmployee, Priority: 100}}})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("cross-tenant assignment error=%v", err)
		}
	})

	t.Run("idempotent replay and key conflict", func(t *testing.T) {
		replay, wasReplay, err := f.service.Create(ctx, f.principalA, CreateInput{MarketplaceKey: "flipkart", OrderIDs: []string{second, first}, IdempotencyKey: "resolved-batch"})
		if err != nil || !wasReplay || replay.ID != created.ID {
			t.Fatalf("replay=%#v replayed=%v err=%v", replay, wasReplay, err)
		}
		_, _, err = f.service.Create(ctx, f.principalA, CreateInput{MarketplaceKey: "flipkart", OrderIDs: []string{unresolved}, IdempotencyKey: "resolved-batch"})
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("key conflict error=%v", err)
		}
	})

	t.Run("duplicate and cross-tenant inclusion are rejected", func(t *testing.T) {
		_, _, err := f.service.Create(ctx, f.principalA, CreateInput{MarketplaceKey: "flipkart", OrderIDs: []string{first}, IdempotencyKey: "duplicate-member"})
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("duplicate error=%v", err)
		}
		_, _, err = f.service.Create(ctx, f.principalA, CreateInput{MarketplaceKey: "flipkart", OrderIDs: []string{otherTenant}, IdempotencyKey: "other-tenant"})
		if !errors.Is(err, ErrIneligible) {
			t.Fatalf("cross-tenant error=%v", err)
		}
		if _, err = f.service.Get(ctx, f.principalB, created.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("cross-tenant get error=%v", err)
		}
	})

	t.Run("state transitions enforce resolution", func(t *testing.T) {
		blocked, _, err := f.service.Create(ctx, f.principalA, CreateInput{MarketplaceKey: "flipkart", OrderIDs: []string{unresolved}, IdempotencyKey: "unresolved-batch"})
		if err != nil || blocked.UnresolvedCount != 1 {
			t.Fatalf("blocked=%#v err=%v", blocked, err)
		}
		if _, err = f.service.Ready(ctx, f.principalA, blocked.ID); !errors.Is(err, ErrUnresolved) {
			t.Fatalf("ready unresolved error=%v", err)
		}
		cancelled, err := f.service.Cancel(ctx, f.principalA, blocked.ID)
		if err != nil || cancelled.Status != "cancelled" || cancelled.CancelledAt == nil {
			t.Fatalf("cancelled=%#v err=%v", cancelled, err)
		}
		if _, err = f.service.Ready(ctx, f.principalA, blocked.ID); !errors.Is(err, ErrInvalidState) {
			t.Fatalf("cancelled to ready error=%v", err)
		}
		ready, err := f.service.Ready(ctx, f.principalA, created.ID)
		if err != nil || ready.Status != "ready" || ready.ReadyAt == nil || len(ready.WorkerTotals) != 1 || ready.WorkerTotals[0].EmployeeID != f.productWorkerID || ready.WorkerTotals[0].TotalQuantity != 5 {
			t.Fatalf("ready=%#v err=%v", ready, err)
		}
		if _, err = f.service.Cancel(ctx, f.principalA, created.ID); !errors.Is(err, ErrInvalidState) {
			t.Fatalf("ready to cancelled error=%v", err)
		}
	})

	t.Run("fallback assignment is snapshotted", func(t *testing.T) {
		var fallbackProduct string
		scanBatchTest(t, f.db, `INSERT INTO products(company_id,internal_code,name) VALUES($1,'P4-FALLBACK','Fallback Product') RETURNING id`, []any{f.companyA}, &fallbackProduct)
		quantity := 4
		orderID := f.order(t, f.companyA, "resolved", &fallbackProduct, &quantity)
		item, _, err := f.service.Create(ctx, f.principalA, CreateInput{MarketplaceKey: "flipkart", OrderIDs: []string{orderID}, IdempotencyKey: "fallback-batch"})
		if err != nil {
			t.Fatal(err)
		}
		ready, err := f.service.Ready(ctx, f.principalA, item.ID)
		if err != nil || len(ready.WorkerTotals) != 1 || ready.WorkerTotals[0].EmployeeID != f.defaultWorkerID || ready.WorkerTotals[0].TotalQuantity != quantity {
			t.Fatalf("fallback ready=%#v err=%v", ready, err)
		}
		_, err = f.service.ReplaceAssignmentRules(ctx, f.principalA, ReplaceAssignmentRulesInput{MarketplaceKey: "flipkart", Rules: []AssignmentRuleInput{{EmployeeID: f.productWorkerID, Priority: 100}, {ProductID: &f.productID, EmployeeID: f.productWorkerID, Priority: 10}}})
		if err != nil {
			t.Fatalf("replace assignments: %v", err)
		}
		historical, err := f.service.Get(ctx, f.principalA, item.ID)
		if err != nil || historical.WorkerTotals[0].EmployeeID != f.defaultWorkerID {
			t.Fatalf("historical assignment changed=%#v err=%v", historical.WorkerTotals, err)
		}
	})

	t.Run("print generation persists ordered traceable artifacts", func(t *testing.T) {
		execBatchTest(t, f.db, `UPDATE marketplace_order_items SET raw_sku=CASE order_id WHEN $1 THEN 'ALPHA' ELSE 'ZETA' END WHERE company_id=$3 AND order_id IN($1,$2)`, first, second, f.companyA)
		execBatchTest(t, f.db, `DELETE FROM role_permissions WHERE company_id=$1 AND role_id=$2 AND permission_key='labels.print'`, f.companyA, f.roleA)
		if _, _, err := f.service.Generate(ctx, f.principalA, created.ID, GenerateInput{IdempotencyKey: "print-denied"}); !errors.Is(err, authorization.ErrPermissionDenied) {
			t.Fatalf("print permission error=%v", err)
		}
		execBatchTest(t, f.db, `INSERT INTO role_permissions(company_id,role_id,permission_key) VALUES($1,$2,'labels.print')`, f.companyA, f.roleA)
		job, replayed, err := f.service.Generate(ctx, f.principalA, created.ID, GenerateInput{ExportInvoices: true, IdempotencyKey: "print-original"})
		if err != nil || replayed || job.Status != "ready" || len(job.Artifacts) != 2 {
			t.Fatalf("job=%#v replayed=%v err=%v", job, replayed, err)
		}
		positions := printPositions(t, f.db, f.companyA, job.ID)
		if len(positions) != 2 || positions[0] != second || positions[1] != first {
			t.Fatalf("original positions=%v", positions)
		}
		replay, wasReplay, err := f.service.Generate(ctx, f.principalA, created.ID, GenerateInput{ExportInvoices: true, IdempotencyKey: "print-original"})
		if err != nil || !wasReplay || replay.ID != job.ID {
			t.Fatalf("replay=%#v replayed=%v err=%v", replay, wasReplay, err)
		}
		sorted, _, err := f.service.Generate(ctx, f.principalA, created.ID, GenerateInput{SortLabels: true, IdempotencyKey: "print-sorted"})
		if err != nil || len(sorted.Artifacts) != 1 {
			t.Fatalf("sorted=%#v err=%v", sorted, err)
		}
		positions = printPositions(t, f.db, f.companyA, sorted.ID)
		if positions[0] != first || positions[1] != second {
			t.Fatalf("sorted positions=%v", positions)
		}
		data, _, err := f.service.DownloadArtifact(ctx, f.principalA, job.Artifacts[0].ID)
		if err != nil || len(data) == 0 {
			t.Fatalf("download bytes=%d err=%v", len(data), err)
		}
		if _, _, err = f.service.DownloadArtifact(ctx, f.principalB, job.Artifacts[0].ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("cross-tenant artifact error=%v", err)
		}
		execBatchTest(t, f.db, `DELETE FROM role_permissions WHERE company_id=$1 AND role_id=$2 AND permission_key='labels.reprint'`, f.companyA, f.roleA)
		if _, _, err = f.service.Reprint(ctx, f.principalA, job.ID, ReprintInput{Reason: "Damaged paper", IdempotencyKey: "reprint-denied"}); !errors.Is(err, authorization.ErrPermissionDenied) {
			t.Fatalf("reprint permission error=%v", err)
		}
		execBatchTest(t, f.db, `INSERT INTO role_permissions(company_id,role_id,permission_key) VALUES($1,$2,'labels.reprint')`, f.companyA, f.roleA)
		var inventoryBefore int
		scanBatchTest(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, []any{f.companyA}, &inventoryBefore)
		reprinted, wasReplay, err := f.service.Reprint(ctx, f.principalA, job.ID, ReprintInput{Reason: "Damaged paper", IdempotencyKey: "reprint-one"})
		if err != nil || wasReplay || reprinted.SourcePrintJobID == nil || *reprinted.SourcePrintJobID != job.ID || reprinted.ReprintReason == nil || *reprinted.ReprintReason != "Damaged paper" || len(reprinted.Artifacts) != 2 {
			t.Fatalf("reprint=%#v replay=%v err=%v", reprinted, wasReplay, err)
		}
		replayedReprint, wasReplay, err := f.service.Reprint(ctx, f.principalA, job.ID, ReprintInput{Reason: "Damaged paper", IdempotencyKey: "reprint-one"})
		if err != nil || !wasReplay || replayedReprint.ID != reprinted.ID {
			t.Fatalf("reprint replay=%#v replay=%v err=%v", replayedReprint, wasReplay, err)
		}
		var inventoryAfter int
		scanBatchTest(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, []any{f.companyA}, &inventoryAfter)
		if inventoryAfter != inventoryBefore {
			t.Fatalf("reprint changed inventory transactions: before=%d after=%d", inventoryBefore, inventoryAfter)
		}
		var reprintAudits int
		scanBatchTest(t, f.db, `SELECT count(*) FROM audit_logs WHERE company_id=$1 AND action='print.reprinted' AND target_id=$2`, []any{f.companyA, reprinted.ID}, &reprintAudits)
		if reprintAudits != 1 {
			t.Fatalf("reprint audits=%d", reprintAudits)
		}
		f.generator.err = errors.New("fixture generation failure")
		if _, _, err = f.service.Generate(ctx, f.principalA, created.ID, GenerateInput{IdempotencyKey: "print-failed"}); !errors.Is(err, ErrGenerationFailed) {
			t.Fatalf("generation failure error=%v", err)
		}
		f.generator.err = nil
		var failedID, failedStatus string
		scanBatchTest(t, f.db, `SELECT id,status FROM print_jobs WHERE company_id=$1 AND idempotency_key='print-failed'`, []any{f.companyA}, &failedID, &failedStatus)
		if failedStatus != "failed" {
			t.Fatalf("failed print status=%s", failedStatus)
		}
		var failureAudits int
		scanBatchTest(t, f.db, `SELECT count(*) FROM audit_logs WHERE company_id=$1 AND target_type='print_job' AND target_id=$2 AND action='print.generation_failed'`, []any{f.companyA, failedID}, &failureAudits)
		if failureAudits != 1 {
			t.Fatalf("failure audit count=%d", failureAudits)
		}
	})

	t.Run("Amazon uses shared batch print artifacts with enrichment inputs", func(t *testing.T) {
		quantity := 4
		orderID := f.amazonOrder(t, quantity)
		eligible, err := f.service.EligibleOrders(ctx, f.principalA, "amazon")
		if err != nil || len(eligible) != 1 || eligible[0].OrderID != orderID || eligible[0].UnresolvedCount != 0 {
			t.Fatalf("Amazon eligible=%#v err=%v", eligible, err)
		}
		created, replayed, err := f.service.Create(ctx, f.principalA, CreateInput{MarketplaceKey: "amazon", OrderIDs: []string{orderID}, IdempotencyKey: "amazon-batch"})
		if err != nil || replayed || created.MarketplaceKey != "amazon" {
			t.Fatalf("Amazon batch=%#v replay=%v err=%v", created, replayed, err)
		}
		ready, err := f.service.Ready(ctx, f.principalA, created.ID)
		if err != nil || ready.Status != "ready" {
			t.Fatalf("Amazon ready=%#v err=%v", ready, err)
		}
		beforeCalls := len(f.generator.calls)
		job, replayed, err := f.service.Generate(ctx, f.principalA, created.ID, GenerateInput{ExportInvoices: true, IdempotencyKey: "amazon-print"})
		if err != nil || replayed || job.Status != "ready" || job.GenerationVersion != "amazon-a4-enriched-v1" || len(job.Artifacts) != 2 || len(f.generator.calls) != beforeCalls+1 {
			t.Fatalf("Amazon print=%#v replay=%v err=%v", job, replayed, err)
		}
		input := f.generator.calls[len(f.generator.calls)-1]
		if len(input) != 1 || input[0].Number != 1 || input[0].InvoiceNumber != 2 || input[0].SKU != "AMAZON-SKU" || input[0].Quantity != quantity {
			t.Fatalf("Amazon generator input=%#v", input)
		}
		var inventoryBefore, inventoryAfter int
		scanBatchTest(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, []any{f.companyA}, &inventoryBefore)
		reprint, wasReplay, err := f.service.Reprint(ctx, f.principalA, job.ID, ReprintInput{Reason: "Unreadable enrichment", IdempotencyKey: "amazon-reprint"})
		if err != nil || wasReplay || reprint.GenerationVersion != "amazon-a4-enriched-v1" {
			t.Fatalf("Amazon reprint=%#v replay=%v err=%v", reprint, wasReplay, err)
		}
		scanBatchTest(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, []any{f.companyA}, &inventoryAfter)
		if inventoryAfter != inventoryBefore {
			t.Fatalf("Amazon print changed inventory: before=%d after=%d", inventoryBefore, inventoryAfter)
		}
		execBatchTest(t, f.db, `UPDATE module_entitlements SET enabled=false WHERE company_id=$1 AND module_key='amazon'`, f.companyA)
		if _, err = f.service.EligibleOrders(ctx, f.principalA, "amazon"); !errors.Is(err, authorization.ErrModuleUnavailable) {
			t.Fatalf("Amazon entitlement error=%v", err)
		}
		execBatchTest(t, f.db, `UPDATE module_entitlements SET enabled=true WHERE company_id=$1 AND module_key='amazon'`, f.companyA)
	})

	var auditCount int
	scanBatchTest(t, f.db, `SELECT count(*) FROM audit_logs WHERE company_id=$1 AND target_type='batch' AND action IN ('batch.created','batch.ready','batch.cancelled')`, []any{f.companyA}, &auditCount)
	if auditCount != 8 {
		t.Fatalf("audit count=%d", auditCount)
	}
}

func TestMeeshoUsesSharedBatchPrintingAndAssignments(t *testing.T) {
	f := setupBatch(t)
	ctx := context.Background()
	quantity := 5
	orderID := f.meeshoOrder(t, quantity)

	eligible, err := f.service.EligibleOrders(ctx, f.principalA, "meesho")
	if err != nil || len(eligible) != 1 || eligible[0].OrderID != orderID || eligible[0].UnresolvedCount != 0 {
		t.Fatalf("eligible=%#v err=%v", eligible, err)
	}
	created, replay, err := f.service.Create(ctx, f.principalA, CreateInput{MarketplaceKey: "meesho", OrderIDs: []string{orderID}, IdempotencyKey: "meesho-batch"})
	if err != nil || replay || created.MarketplaceKey != "meesho" || len(created.ProductTotals) != 1 || created.ProductTotals[0].TotalQuantity != quantity {
		t.Fatalf("created=%#v replay=%v err=%v", created, replay, err)
	}
	ready, err := f.service.Ready(ctx, f.principalA, created.ID)
	if err != nil || ready.Status != "ready" || len(ready.WorkerTotals) != 1 || ready.WorkerTotals[0].EmployeeID != f.productWorkerID {
		t.Fatalf("ready=%#v err=%v", ready, err)
	}
	var inventoryBefore, inventoryAfter int
	scanBatchTest(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, []any{f.companyA}, &inventoryBefore)
	job, replay, err := f.service.Generate(ctx, f.principalA, created.ID, GenerateInput{IdempotencyKey: "meesho-print"})
	if err != nil || replay || job.Status != "ready" || job.GenerationVersion != pdfgenerator.SourcePageGenerationVersion || len(job.Artifacts) != 1 {
		t.Fatalf("print=%#v replay=%v err=%v", job, replay, err)
	}
	input := f.generator.calls[len(f.generator.calls)-1]
	if len(input) != 1 || input[0].Number != 1 || input[0].InvoiceNumber != 0 || input[0].SKU != "MEESHO-SKU" || input[0].Quantity != quantity {
		t.Fatalf("generator input=%#v", input)
	}
	reprint, replay, err := f.service.Reprint(ctx, f.principalA, job.ID, ReprintInput{Reason: "Carrier label damaged", IdempotencyKey: "meesho-reprint"})
	if err != nil || replay || reprint.GenerationVersion != pdfgenerator.SourcePageGenerationVersion || reprint.SourcePrintJobID == nil || *reprint.SourcePrintJobID != job.ID {
		t.Fatalf("reprint=%#v replay=%v err=%v", reprint, replay, err)
	}
	scanBatchTest(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, []any{f.companyA}, &inventoryAfter)
	if inventoryAfter != inventoryBefore {
		t.Fatalf("print/reprint changed inventory: before=%d after=%d", inventoryBefore, inventoryAfter)
	}
	var traceable bool
	scanBatchTest(t, f.db, `SELECT i.marketplace_order_id=$1 AND i.source_file_id=$2 AND i.processing_job_id=$3 AND i.source_page=1 FROM print_job_items i WHERE i.company_id=$4 AND i.print_job_id=$5`, []any{orderID, created.Members[0].SourceFileID, created.Members[0].ProcessingJobID, f.companyA, job.ID}, &traceable)
	if !traceable {
		t.Fatal("Meesho print traceability was not preserved")
	}
}

func TestSnapdealUsesSharedBatchPrintingAndInvoiceAssociation(t *testing.T) {
	f := setupBatch(t)
	ctx := context.Background()
	quantity := 2
	seed := fmt.Sprintf("snapdeal-%s-%d", f.companyA, time.Now().UnixNano())
	hash := sha256.Sum256([]byte(seed))
	var source, job, order string
	scanBatchTest(t, f.db, `INSERT INTO source_files(company_id,marketplace_key,storage_key,original_filename,content_type,size_bytes,sha256,uploaded_by) VALUES($1,'snapdeal',$2,'snapdeal.pdf','application/pdf',1,$3,$4) RETURNING id`, []any{f.companyA, seed, hex.EncodeToString(hash[:]), f.userID}, &source)
	pdf := []byte("%PDF-snapdeal-source")
	if err := f.storage.Put(ctx, seed, bytes.NewReader(pdf), int64(len(pdf)), "application/pdf"); err != nil {
		t.Fatal(err)
	}
	scanBatchTest(t, f.db, `INSERT INTO processing_jobs(company_id,source_file_id,marketplace_key,status,parser_version,total_pages,processed_pages) VALUES($1,$2,'snapdeal','processed','snapdeal-packslip-v1',2,2) RETURNING id`, []any{f.companyA, source}, &job)
	scanBatchTest(t, f.db, `INSERT INTO marketplace_orders(company_id,marketplace_key,source_file_id,processing_job_id,source_page,marketplace_order_id,status,parser_version) VALUES($1,'snapdeal',$2,$3,1,'88000000999','resolved','snapdeal-packslip-v1') RETURNING id`, []any{f.companyA, source, job}, &order)
	execBatchTest(t, f.db, `INSERT INTO marketplace_order_documents(company_id,order_id,source_file_id,source_page,document_role,extraction_method) VALUES($1,$2,$3,1,'shipping_label','text'),($1,$2,$3,2,'invoice','text')`, f.companyA, order, source)
	execBatchTest(t, f.db, `INSERT INTO marketplace_order_items(company_id,order_id,raw_sku,product_id,quantity,quantity_source,resolution_status) VALUES($1,$2,'9_SAFE-SKU',$3,$4,'extracted','resolved')`, f.companyA, order, f.productID, quantity)
	eligible, err := f.service.EligibleOrders(ctx, f.principalA, "snapdeal")
	if err != nil || len(eligible) != 1 {
		t.Fatalf("eligible=%#v err=%v", eligible, err)
	}
	batch, _, err := f.service.Create(ctx, f.principalA, CreateInput{MarketplaceKey: "snapdeal", OrderIDs: []string{order}, IdempotencyKey: "snapdeal-batch"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Ready(ctx, f.principalA, batch.ID); err != nil {
		t.Fatal(err)
	}
	var before, after int
	scanBatchTest(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, []any{f.companyA}, &before)
	printJob, replay, err := f.service.Generate(ctx, f.principalA, batch.ID, GenerateInput{ExportInvoices: true, IdempotencyKey: "snapdeal-print"})
	if err != nil || replay || printJob.GenerationVersion != "snapdeal-packslip-enriched-v1" || len(printJob.Artifacts) != 2 {
		t.Fatalf("print=%#v replay=%v err=%v", printJob, replay, err)
	}
	input := f.generator.calls[len(f.generator.calls)-1]
	if len(input) != 1 || input[0].Number != 1 || input[0].InvoiceNumber != 2 || input[0].SKU != "9_SAFE-SKU" || input[0].Quantity != quantity {
		t.Fatalf("input=%#v", input)
	}
	if _, _, err = f.service.Reprint(ctx, f.principalA, printJob.ID, ReprintInput{Reason: "Damaged label", IdempotencyKey: "snapdeal-reprint"}); err != nil {
		t.Fatal(err)
	}
	scanBatchTest(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, []any{f.companyA}, &after)
	if after != before {
		t.Fatalf("print changed inventory %d -> %d", before, after)
	}
}

func TestBatchMigrationUpDown(t *testing.T) {
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
	schema := "p4_migration_" + fmt.Sprint(time.Now().UnixNano())
	if _, err = tx.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SET LOCAL search_path TO `+schema+`,public`); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("..", "..", "migrations")
	for _, name := range []string{"000001_core_platform.up.sql", "000002_tenant_sessions.up.sql", "000003_product_master.up.sql", "000004_flipkart_processing.up.sql", "000005_flipkart_worker_leases.up.sql", "000006_batch_foundation.up.sql", "000007_print_generation.up.sql", "000008_worker_assignments_reprints.up.sql"} {
		sql, readErr := os.ReadFile(filepath.Join(root, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = tx.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	var exists *string
	if err = tx.QueryRow(ctx, `SELECT to_regclass($1)`, schema+".batch_worker_assignments").Scan(&exists); err != nil || exists == nil {
		t.Fatalf("up verification=%v err=%v", exists, err)
	}
	down, err := os.ReadFile(filepath.Join(root, "000008_worker_assignments_reprints.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(down)); err != nil {
		t.Fatalf("down: %v", err)
	}
	exists = nil
	if err = tx.QueryRow(ctx, `SELECT to_regclass($1)`, schema+".batch_worker_assignments").Scan(&exists); err != nil || exists != nil {
		t.Fatalf("down verification=%v err=%v", exists, err)
	}
	down, err = os.ReadFile(filepath.Join(root, "000007_print_generation.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(down)); err != nil {
		t.Fatalf("print down: %v", err)
	}
	down, err = os.ReadFile(filepath.Join(root, "000006_batch_foundation.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(down)); err != nil {
		t.Fatalf("batch foundation down: %v", err)
	}
	exists = nil
	if err = tx.QueryRow(ctx, `SELECT to_regclass($1)`, schema+".batches").Scan(&exists); err != nil || exists != nil {
		t.Fatalf("batch down verification=%v err=%v", exists, err)
	}
}

func execBatchTest(t *testing.T, db *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("fixture exec: %v", err)
	}
}

func scanBatchTest(t *testing.T, db *pgxpool.Pool, query string, args []any, dest ...any) {
	t.Helper()
	if err := db.QueryRow(context.Background(), query, args...).Scan(dest...); err != nil {
		t.Fatalf("fixture scan: %v", err)
	}
}

func printPositions(t *testing.T, db *pgxpool.Pool, companyID, jobID string) []string {
	t.Helper()
	rows, err := db.Query(context.Background(), `SELECT marketplace_order_id FROM print_job_items WHERE company_id=$1 AND print_job_id=$2 ORDER BY output_position`, companyID, jobID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	positions := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		positions = append(positions, id)
	}
	return positions
}

func cleanupBatch(t *testing.T, f *batchFixture) {
	t.Helper()
	companies := []string{f.companyA, f.companyB}
	for _, table := range []string{"print_artifacts", "print_job_items", "print_jobs", "batch_worker_assignments", "worker_assignment_rules", "batch_members", "batches", "marketplace_order_documents", "marketplace_order_items", "marketplace_orders", "processing_jobs", "source_files", "products", "audit_logs", "module_entitlements", "company_user_roles", "role_permissions", "employees", "roles", "company_users"} {
		query := "DELETE FROM " + table + " WHERE company_id=ANY($1::uuid[])"
		execBatchTest(t, f.db, query, companies)
	}
	_, _ = f.db.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, f.userID)
	_, _ = f.db.Exec(context.Background(), `DELETE FROM companies WHERE id=ANY($1::uuid[])`, companies)
}
