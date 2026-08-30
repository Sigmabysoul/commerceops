package reporting

import (
	"context"
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

func TestDashboardAuthoritativeTotalsBoundariesAndTenantIsolation(t *testing.T) {
	db := reportingDB(t)
	ctx := context.Background()
	suffix := fmt.Sprint(time.Now().UnixNano())
	var company, other, user, role, product, source, job, order string
	mustScan(t, db, `INSERT INTO companies(name) VALUES($1) RETURNING id`, []any{"Reports A " + suffix}, &company)
	mustScan(t, db, `INSERT INTO companies(name) VALUES($1) RETURNING id`, []any{"Reports B " + suffix}, &other)
	mustScan(t, db, `INSERT INTO users(email,password_hash) VALUES($1,'test') RETURNING id`, []any{"reports-" + suffix + "@example.test"}, &user)
	mustExec(t, db, `INSERT INTO company_users(company_id,user_id) VALUES($1,$3),($2,$3)`, company, other, user)
	mustScan(t, db, `INSERT INTO roles(company_id,name) VALUES($1,'Reporter') RETURNING id`, []any{company}, &role)
	mustExec(t, db, `INSERT INTO role_permissions(company_id,role_id,permission_key) VALUES($1,$2,'reports.view'),($1,$2,'inventory.view')`, company, role)
	mustExec(t, db, `INSERT INTO company_user_roles(company_id,user_id,role_id) VALUES($1,$2,$3)`, company, user, role)
	mustExec(t, db, `INSERT INTO module_entitlements(company_id,module_key,enabled) VALUES($1,'inventory',true)`, company)
	mustScan(t, db, `INSERT INTO products(company_id,internal_code,name) VALUES($1,'REP-1','Report product') RETURNING id`, []any{company}, &product)
	mustScan(t, db, `INSERT INTO source_files(company_id,marketplace_key,storage_key,original_filename,content_type,size_bytes,sha256,uploaded_by) VALUES($1,'flipkart',$2,'report.pdf','application/pdf',1,$3,$4) RETURNING id`, []any{company, "reports/" + suffix, fmt.Sprintf("%064x", time.Now().UnixNano()), user}, &source)
	mustScan(t, db, `INSERT INTO processing_jobs(company_id,source_file_id,marketplace_key,status,parser_version,created_at) VALUES($1,$2,'flipkart','processed','test',$3) RETURNING id`, []any{company, source, time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)}, &job)
	mustScan(t, db, `INSERT INTO marketplace_orders(company_id,marketplace_key,source_file_id,processing_job_id,source_page,marketplace_order_id,status,parser_version,created_at) VALUES($1,'flipkart',$2,$3,1,$4,'resolved','test',$5) RETURNING id`, []any{company, source, job, "ORDER-" + suffix, time.Date(2026, 8, 30, 5, 30, 0, 0, time.UTC)}, &order)
	mustExec(t, db, `INSERT INTO marketplace_order_items(company_id,order_id,raw_sku,product_id,quantity,quantity_source,resolution_status) VALUES($1,$2,'REP-SKU',$3,3,'extracted','resolved')`, company, order, product)
	mustExec(t, db, `INSERT INTO inventory_balances(company_id,product_id,on_hand,reserved) VALUES($1,$2,12,2)`, company, product)
	mustExec(t, db, `INSERT INTO inventory_transactions(company_id,product_id,transaction_type,quantity_delta,previous_balance,resulting_balance,reason,actor_user_id,idempotency_key,request_hash,created_at) VALUES($1,$2,'stock_in',12,0,12,'Report fixture',$3,$4,$5,$6)`, company, product, user, "report-"+suffix, fmt.Sprintf("%064x", time.Now().UnixNano()+1), time.Date(2026, 8, 30, 5, 30, 0, 0, time.UTC))

	service := NewService(db, authorization.NewService(db))
	principal := auth.Principal{CompanyID: company, UserID: user}
	report, err := service.Dashboard(ctx, principal, Filter{From: time.Date(2026, 8, 30, 0, 0, 0, 0, time.FixedZone("IST", 19800)), To: time.Date(2026, 8, 31, 0, 0, 0, 0, time.FixedZone("IST", 19800)), Limit: 50})
	if err != nil || report.Summary.OrdersProcessed != 1 || len(report.ProductQuantities) != 1 || report.ProductQuantities[0].Quantity != 3 || report.Inventory == nil || report.Inventory.StockIn != 12 || report.Inventory.CurrentAvailable != 10 {
		t.Fatalf("report=%#v err=%v", report, err)
	}
	otherReport, err := service.Dashboard(ctx, auth.Principal{CompanyID: other, UserID: user}, Filter{From: report.From, To: report.To, Limit: 50})
	if !errors.Is(err, authorization.ErrPermissionDenied) || otherReport.Summary.OrdersProcessed != 0 {
		t.Fatalf("cross tenant report=%#v err=%v", otherReport, err)
	}
	mustExec(t, db, `DELETE FROM role_permissions WHERE company_id=$1 AND role_id=$2 AND permission_key='inventory.view'`, company, role)
	report, err = service.Dashboard(ctx, principal, Filter{From: report.From, To: report.To, Limit: 50})
	if err != nil || report.InventoryAccess || report.Inventory != nil || len(report.ProductMovements) != 0 || len(report.ProductQuantities) != 1 {
		t.Fatalf("restricted inventory report=%#v err=%v", report, err)
	}
	if _, err = service.Dashboard(ctx, principal, Filter{From: report.To, To: report.From, Limit: 50}); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("invalid range=%v", err)
	}
}

func TestReportingMigrationUpDown(t *testing.T) {
	db := reportingDB(t)
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	schema := "p6_migration_" + fmt.Sprint(time.Now().UnixNano())
	if _, err = tx.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SET LOCAL search_path TO `+schema+`,public`); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("..", "..", "migrations")
	for _, name := range []string{"000001_core_platform.up.sql", "000002_tenant_sessions.up.sql", "000003_product_master.up.sql", "000004_flipkart_processing.up.sql", "000005_flipkart_worker_leases.up.sql", "000006_batch_foundation.up.sql", "000007_print_generation.up.sql", "000008_worker_assignments_reprints.up.sql", "000009_inventory_ledger.up.sql", "000010_inventory_outbound_reservations.up.sql", "000011_dashboard_reporting.up.sql"} {
		data, readErr := os.ReadFile(filepath.Join(root, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = tx.Exec(ctx, string(data)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	var exists *string
	if err = tx.QueryRow(ctx, `SELECT to_regclass($1)`, schema+".inventory_transactions_company_created_idx").Scan(&exists); err != nil || exists == nil {
		t.Fatalf("up=%v err=%v", exists, err)
	}
	down, err := os.ReadFile(filepath.Join(root, "000011_dashboard_reporting.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	exists = nil
	if err = tx.QueryRow(ctx, `SELECT to_regclass($1)`, schema+".inventory_transactions_company_created_idx").Scan(&exists); err != nil || exists != nil {
		t.Fatalf("down=%v err=%v", exists, err)
	}
}

func reportingDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	db, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return db
}
func mustExec(t *testing.T, db *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := db.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}
func mustScan(t *testing.T, db *pgxpool.Pool, sql string, args []any, destinations ...any) {
	t.Helper()
	if err := db.QueryRow(context.Background(), sql, args...).Scan(destinations...); err != nil {
		t.Fatal(err)
	}
}
