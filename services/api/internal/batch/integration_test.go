package batch

import (
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
	"github.com/jackc/pgx/v5/pgxpool"
)

type batchFixture struct {
	db                         *pgxpool.Pool
	service                    *Service
	companyA, companyB, userID string
	roleA, productID           string
	principalA, principalB     auth.Principal
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
		execBatchTest(t, db, `INSERT INTO role_permissions(company_id,role_id,permission_key) VALUES($1,$2,'labels.process')`, company, role)
		execBatchTest(t, db, `INSERT INTO company_user_roles(company_id,user_id,role_id) VALUES($1,$2,$3)`, company, f.userID, role)
		execBatchTest(t, db, `INSERT INTO module_entitlements(company_id,module_key,enabled) VALUES($1,'flipkart',true)`, company)
		if company == f.companyA {
			f.roleA = role
		}
	}
	scanBatchTest(t, db, `INSERT INTO products(company_id,internal_code,name) VALUES($1,'P4-PRODUCT','Phase 4 Product') RETURNING id`, []any{f.companyA}, &f.productID)
	f.service = NewService(db, authorization.NewService(db))
	f.principalA = auth.Principal{CompanyID: f.companyA, UserID: f.userID}
	f.principalB = auth.Principal{CompanyID: f.companyB, UserID: f.userID}
	t.Cleanup(func() { cleanupBatch(t, f); db.Close() })
	return f
}

func (f *batchFixture) order(t *testing.T, company, status string, productID *string, quantity *int) string {
	t.Helper()
	seed := fmt.Sprintf("%s-%d", company, time.Now().UnixNano())
	hash := sha256.Sum256([]byte(seed))
	sha := hex.EncodeToString(hash[:])
	var sourceID, jobID, orderID string
	scanBatchTest(t, f.db, `INSERT INTO source_files(company_id,marketplace_key,storage_key,original_filename,content_type,size_bytes,sha256,uploaded_by) VALUES($1,'flipkart',$2,'batch.pdf','application/pdf',1,$3,$4) RETURNING id`, []any{company, seed, sha, f.userID}, &sourceID)
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
		if err != nil || ready.Status != "ready" || ready.ReadyAt == nil {
			t.Fatalf("ready=%#v err=%v", ready, err)
		}
		if _, err = f.service.Cancel(ctx, f.principalA, created.ID); !errors.Is(err, ErrInvalidState) {
			t.Fatalf("ready to cancelled error=%v", err)
		}
	})

	var auditCount int
	scanBatchTest(t, f.db, `SELECT count(*) FROM audit_logs WHERE company_id=$1 AND target_type='batch' AND action IN ('batch.created','batch.ready','batch.cancelled')`, []any{f.companyA}, &auditCount)
	if auditCount != 4 {
		t.Fatalf("audit count=%d", auditCount)
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
	for _, name := range []string{"000001_core_platform.up.sql", "000002_tenant_sessions.up.sql", "000003_product_master.up.sql", "000004_flipkart_processing.up.sql", "000005_flipkart_worker_leases.up.sql", "000006_batch_foundation.up.sql"} {
		sql, readErr := os.ReadFile(filepath.Join(root, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = tx.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	var exists *string
	if err = tx.QueryRow(ctx, `SELECT to_regclass($1)`, schema+".batches").Scan(&exists); err != nil || exists == nil {
		t.Fatalf("up verification=%v err=%v", exists, err)
	}
	down, err := os.ReadFile(filepath.Join(root, "000006_batch_foundation.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(down)); err != nil {
		t.Fatalf("down: %v", err)
	}
	exists = nil
	if err = tx.QueryRow(ctx, `SELECT to_regclass($1)`, schema+".batches").Scan(&exists); err != nil || exists != nil {
		t.Fatalf("down verification=%v err=%v", exists, err)
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

func cleanupBatch(t *testing.T, f *batchFixture) {
	t.Helper()
	companies := []string{f.companyA, f.companyB}
	for _, table := range []string{"batch_members", "batches", "marketplace_order_items", "marketplace_orders", "processing_jobs", "source_files", "products", "audit_logs", "module_entitlements", "company_user_roles", "role_permissions", "employees", "roles", "company_users"} {
		query := "DELETE FROM " + table + " WHERE company_id=ANY($1::uuid[])"
		execBatchTest(t, f.db, query, companies)
	}
	_, _ = f.db.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, f.userID)
	_, _ = f.db.Exec(context.Background(), `DELETE FROM companies WHERE id=ANY($1::uuid[])`, companies)
}
