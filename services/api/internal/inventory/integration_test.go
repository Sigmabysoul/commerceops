package inventory

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	db                                                                      *pgxpool.Pool
	service                                                                 *Service
	principal                                                               auth.Principal
	company, otherCompany, user, role, product, secondProduct, otherProduct string
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
	scan(t, db, `INSERT INTO companies(name) VALUES($1) RETURNING id`, []any{"Inventory A " + suffix}, &f.company)
	scan(t, db, `INSERT INTO companies(name) VALUES($1) RETURNING id`, []any{"Inventory B " + suffix}, &f.otherCompany)
	scan(t, db, `INSERT INTO users(email,password_hash) VALUES($1,'test') RETURNING id`, []any{"inventory-" + suffix + "@example.test"}, &f.user)
	exec(t, db, `INSERT INTO company_users(company_id,user_id) VALUES($1,$3),($2,$3)`, f.company, f.otherCompany, f.user)
	scan(t, db, `INSERT INTO roles(company_id,name) VALUES($1,'Inventory Operator') RETURNING id`, []any{f.company}, &f.role)
	exec(t, db, `INSERT INTO role_permissions(company_id,role_id,permission_key) VALUES($1,$2,'inventory.view'),($1,$2,'inventory.stock_in'),($1,$2,'inventory.adjust'),($1,$2,'inventory.dispatch')`, f.company, f.role)
	exec(t, db, `INSERT INTO company_user_roles(company_id,user_id,role_id) VALUES($1,$2,$3)`, f.company, f.user, f.role)
	exec(t, db, `INSERT INTO module_entitlements(company_id,module_key,enabled) VALUES($1,'inventory',true)`, f.company)
	scan(t, db, `INSERT INTO products(company_id,internal_code,name) VALUES($1,'INV-A','Inventory A') RETURNING id`, []any{f.company}, &f.product)
	scan(t, db, `INSERT INTO products(company_id,internal_code,name) VALUES($1,'INV-A2','Inventory A2') RETURNING id`, []any{f.company}, &f.secondProduct)
	scan(t, db, `INSERT INTO products(company_id,internal_code,name) VALUES($1,'INV-B','Inventory B') RETURNING id`, []any{f.otherCompany}, &f.otherProduct)
	f.principal = auth.Principal{CompanyID: f.company, UserID: f.user}
	f.service = NewService(db, authorization.NewService(db))
	t.Cleanup(db.Close)
	return f
}

func TestInventoryPostgreSQLBehavior(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	created, replay, err := f.service.StockIn(ctx, f.principal, CommandInput{ProductID: f.product, Quantity: 10, Reason: "Opening stock", IdempotencyKey: "stock-in-one"})
	if err != nil || replay || created.PreviousBalance != 0 || created.ResultingBalance != 10 {
		t.Fatalf("stock in=%#v replay=%v err=%v", created, replay, err)
	}
	replayed, wasReplay, err := f.service.StockIn(ctx, f.principal, CommandInput{ProductID: f.product, Quantity: 10, Reason: "Opening stock", IdempotencyKey: "stock-in-one"})
	if err != nil || !wasReplay || replayed.ID != created.ID {
		t.Fatalf("replay=%#v replay=%v err=%v", replayed, wasReplay, err)
	}
	if _, _, err = f.service.StockIn(ctx, f.principal, CommandInput{ProductID: f.product, Quantity: 11, Reason: "Opening stock", IdempotencyKey: "stock-in-one"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflict=%v", err)
	}
	if _, _, err = f.service.Adjust(ctx, f.principal, CommandInput{ProductID: f.product, Quantity: -1, IdempotencyKey: "missing-reason"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing reason=%v", err)
	}
	if _, _, err = f.service.StockIn(ctx, f.principal, CommandInput{ProductID: f.otherProduct, Quantity: 1, Reason: "Cross tenant", IdempotencyKey: "cross-tenant"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross tenant=%v", err)
	}
	exec(t, f.db, `DELETE FROM role_permissions WHERE company_id=$1 AND role_id=$2 AND permission_key='inventory.adjust'`, f.company, f.role)
	if _, _, err = f.service.Adjust(ctx, f.principal, CommandInput{ProductID: f.product, Quantity: -1, Reason: "Denied", IdempotencyKey: "denied"}); !errors.Is(err, authorization.ErrPermissionDenied) {
		t.Fatalf("permission=%v", err)
	}
	exec(t, f.db, `INSERT INTO role_permissions(company_id,role_id,permission_key) VALUES($1,$2,'inventory.adjust')`, f.company, f.role)

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for index := 0; index < 2; index++ {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			<-start
			_, _, runErr := f.service.Adjust(ctx, f.principal, CommandInput{ProductID: f.product, Quantity: -7, Reason: "Concurrent dispatch simulation", IdempotencyKey: key})
			results <- runErr
		}(fmt.Sprintf("concurrent-%d", index))
	}
	close(start)
	wg.Wait()
	close(results)
	success, insufficient := 0, 0
	for result := range results {
		if result == nil {
			success++
		} else if errors.Is(result, ErrInsufficientStock) {
			insufficient++
		} else {
			t.Fatalf("concurrent error=%v", result)
		}
	}
	if success != 1 || insufficient != 1 {
		t.Fatalf("concurrent success=%d insufficient=%d", success, insufficient)
	}
	balances, err := f.service.ListBalances(ctx, f.principal)
	if err != nil || len(balances) != 2 || balances[0].OnHand != 3 {
		t.Fatalf("balances=%#v err=%v", balances, err)
	}
	if _, _, err = f.service.Adjust(ctx, f.principal, CommandInput{ProductID: f.product, Quantity: -4, Reason: "Would go negative", IdempotencyKey: "negative"}); !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("negative=%v", err)
	}
	correction, _, err := f.service.Correct(ctx, f.principal, CommandInput{ProductID: f.product, Quantity: 2, Reason: "Correct receiving count", ReferenceType: stringPtr("inventory_transaction"), ReferenceID: &created.ID, IdempotencyKey: "correction"})
	if err != nil || correction.ResultingBalance != 5 {
		t.Fatalf("correction=%#v err=%v", correction, err)
	}
	transactions, err := f.service.ListTransactions(ctx, f.principal, f.product, "")
	if err != nil || len(transactions) != 3 {
		t.Fatalf("transactions=%d err=%v", len(transactions), err)
	}
	var audits int
	scan(t, f.db, `SELECT count(*) FROM audit_logs WHERE company_id=$1 AND target_type='inventory_transaction'`, []any{f.company}, &audits)
	if audits != 3 {
		t.Fatalf("audits=%d", audits)
	}
	if _, err = f.db.Exec(ctx, `UPDATE inventory_transactions SET reason='rewrite' WHERE company_id=$1 AND id=$2`, f.company, created.ID); err == nil {
		t.Fatal("ledger update was allowed")
	}
	if _, err = f.db.Exec(ctx, `DELETE FROM inventory_transactions WHERE company_id=$1 AND id=$2`, f.company, created.ID); err == nil {
		t.Fatal("ledger delete was allowed")
	}
}

func TestInventoryMigrationUpDown(t *testing.T) {
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
	schema := "p5_migration_" + fmt.Sprint(time.Now().UnixNano())
	exec(t, tx, `CREATE SCHEMA `+schema)
	exec(t, tx, `SET LOCAL search_path TO `+schema+`,public`)
	root := filepath.Join("..", "..", "migrations")
	for _, name := range []string{"000001_core_platform.up.sql", "000002_tenant_sessions.up.sql", "000003_product_master.up.sql", "000004_flipkart_processing.up.sql", "000005_flipkart_worker_leases.up.sql", "000006_batch_foundation.up.sql", "000007_print_generation.up.sql", "000008_worker_assignments_reprints.up.sql", "000009_inventory_ledger.up.sql", "000010_inventory_outbound_reservations.up.sql"} {
		data, readErr := os.ReadFile(filepath.Join(root, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = tx.Exec(ctx, string(data)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	var exists *string
	if err = tx.QueryRow(ctx, `SELECT to_regclass($1)`, schema+".inventory_transactions").Scan(&exists); err != nil || exists == nil {
		t.Fatalf("up=%v err=%v", exists, err)
	}
	down, err := os.ReadFile(filepath.Join(root, "000010_inventory_outbound_reservations.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(down)); err != nil {
		t.Fatalf("down: %v", err)
	}
	exists = nil
	if err = tx.QueryRow(ctx, `SELECT to_regclass($1)`, schema+".inventory_reservations").Scan(&exists); err != nil || exists != nil {
		t.Fatalf("down=%v err=%v", exists, err)
	}
}

func TestInventoryReasonableLoad(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	const count = 250
	for index := 0; index < count; index++ {
		if _, _, err := f.service.StockIn(ctx, f.principal, CommandInput{ProductID: f.product, Quantity: 1, Reason: "Load fixture", IdempotencyKey: fmt.Sprintf("load-%d", index)}); err != nil {
			t.Fatalf("transaction %d: %v", index, err)
		}
	}
	balances, err := f.service.ListBalances(ctx, f.principal)
	if err != nil || balances[0].OnHand != count {
		t.Fatalf("balance=%#v err=%v", balances, err)
	}
	var transactions int
	scan(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1 AND product_id=$2`, []any{f.company, f.product}, &transactions)
	if transactions != count {
		t.Fatalf("transactions=%d", transactions)
	}
}

func TestReservationLifecycle(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, _, err := f.service.StockIn(ctx, f.principal, CommandInput{ProductID: f.product, Quantity: 10, Reason: "Reservation stock", IdempotencyKey: "reserve-stock"}); err != nil {
		t.Fatal(err)
	}
	r, replay, err := f.service.Reserve(ctx, f.principal, ReserveInput{ProductID: f.product, Quantity: 4, Reason: "Future consignment", SourceType: "consignment", SourceID: "draft-1", IdempotencyKey: "reserve-1"})
	if err != nil || replay || r.Status != "active" {
		t.Fatalf("reserve=%#v replay=%v err=%v", r, replay, err)
	}
	if _, _, err = f.service.Reserve(ctx, f.principal, ReserveInput{ProductID: f.product, Quantity: 7, Reason: "Too much", SourceType: "consignment", SourceID: "draft-2", IdempotencyKey: "reserve-too-much"}); !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("over reserve=%v", err)
	}
	balances, err := f.service.ListBalances(ctx, f.principal)
	if err != nil || balances[0].OnHand != 10 || balances[0].Reserved != 4 || balances[0].Available != 6 {
		t.Fatalf("reserved balance=%#v err=%v", balances, err)
	}
	released, replay, err := f.service.ReleaseReservation(ctx, f.principal, r.ID, ReleaseInput{Reason: "Plan cancelled", IdempotencyKey: "release-1"})
	if err != nil || replay || released.Status != "released" {
		t.Fatalf("release=%#v replay=%v err=%v", released, replay, err)
	}
	_, replay, err = f.service.ReleaseReservation(ctx, f.principal, r.ID, ReleaseInput{Reason: "Plan cancelled", IdempotencyKey: "release-1"})
	if err != nil || !replay {
		t.Fatalf("release replay=%v err=%v", replay, err)
	}
	balances, _ = f.service.ListBalances(ctx, f.principal)
	if balances[0].Reserved != 0 || balances[0].Available != 10 {
		t.Fatalf("released balance=%#v", balances)
	}
}

func TestEcommerceOutboundAtomicAndIdempotent(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	exec(t, f.db, `INSERT INTO module_entitlements(company_id,module_key,enabled) VALUES($1,'flipkart',true)`, f.company)
	var source, job, order, batch string
	scan(t, f.db, `INSERT INTO source_files(company_id,marketplace_key,storage_key,original_filename,content_type,size_bytes,sha256,uploaded_by) VALUES($1,'flipkart',$2,'outbound.pdf','application/pdf',1,$3,$4) RETURNING id`, []any{f.company, "outbound-" + f.company, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", f.user}, &source)
	scan(t, f.db, `INSERT INTO processing_jobs(company_id,source_file_id,marketplace_key,status,parser_version,total_pages,processed_pages) VALUES($1,$2,'flipkart','processed','test',1,1) RETURNING id`, []any{f.company, source}, &job)
	scan(t, f.db, `INSERT INTO marketplace_orders(company_id,marketplace_key,source_file_id,processing_job_id,source_page,marketplace_order_id,status,parser_version) VALUES($1,'flipkart',$2,$3,1,$4,'resolved','test') RETURNING id`, []any{f.company, source, job, "OUT-" + f.company}, &order)
	exec(t, f.db, `INSERT INTO marketplace_order_items(company_id,order_id,product_id,quantity,quantity_source,resolution_status) VALUES($1,$2,$3,6,'extracted','resolved')`, f.company, order, f.product)
	exec(t, f.db, `INSERT INTO marketplace_order_items(company_id,order_id,product_id,quantity,quantity_source,resolution_status) VALUES($1,$2,$3,3,'extracted','resolved')`, f.company, order, f.secondProduct)
	scan(t, f.db, `INSERT INTO batches(company_id,marketplace_key,status,created_by,idempotency_key,request_hash,ready_at) VALUES($1,'flipkart','ready',$2,$3,$4,now()) RETURNING id`, []any{f.company, f.user, "outbound-batch-" + f.company, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}, &batch)
	exec(t, f.db, `INSERT INTO batch_members(company_id,batch_id,marketplace_order_id,position) VALUES($1,$2,$3,1)`, f.company, batch, order)
	if _, _, err := f.service.StockIn(ctx, f.principal, CommandInput{ProductID: f.product, Quantity: 10, Reason: "Dispatch stock", IdempotencyKey: "dispatch-stock"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.service.StockIn(ctx, f.principal, CommandInput{ProductID: f.secondProduct, Quantity: 10, Reason: "Dispatch stock", IdempotencyKey: "dispatch-stock-2"}); err != nil {
		t.Fatal(err)
	}
	items, replay, err := f.service.ConfirmEcommerceOutbound(ctx, f.principal, batch, OutboundInput{IdempotencyKey: "dispatch-1"})
	if err != nil || replay || len(items) != 2 {
		t.Fatalf("outbound=%#v replay=%v err=%v", items, replay, err)
	}
	items, replay, err = f.service.ConfirmEcommerceOutbound(ctx, f.principal, batch, OutboundInput{IdempotencyKey: "dispatch-1"})
	if err != nil || !replay || len(items) != 2 {
		t.Fatalf("outbound replay=%#v replay=%v err=%v", items, replay, err)
	}
	var count int
	scan(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1 AND transaction_type='ecommerce_out'`, []any{f.company}, &count)
	if count != 2 {
		t.Fatalf("outbound count=%d", count)
	}
	var order2, batch2 string
	scan(t, f.db, `INSERT INTO marketplace_orders(company_id,marketplace_key,source_file_id,processing_job_id,source_page,marketplace_order_id,status,parser_version) VALUES($1,'flipkart',$2,$3,2,$4,'resolved','test') RETURNING id`, []any{f.company, source, job, "OUT2-" + f.company}, &order2)
	exec(t, f.db, `INSERT INTO marketplace_order_items(company_id,order_id,product_id,quantity,quantity_source,resolution_status) VALUES($1,$2,$3,1,'extracted','resolved'),($1,$2,$4,8,'extracted','resolved')`, f.company, order2, f.product, f.secondProduct)
	scan(t, f.db, `INSERT INTO batches(company_id,marketplace_key,status,created_by,idempotency_key,request_hash,ready_at) VALUES($1,'flipkart','ready',$2,$3,$4,now()) RETURNING id`, []any{f.company, f.user, "outbound-batch-2-" + f.company, "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}, &batch2)
	exec(t, f.db, `INSERT INTO batch_members(company_id,batch_id,marketplace_order_id,position) VALUES($1,$2,$3,1)`, f.company, batch2, order2)
	if _, _, err = f.service.ConfirmEcommerceOutbound(ctx, f.principal, batch2, OutboundInput{IdempotencyKey: "dispatch-insufficient"}); !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("atomic insufficient=%v", err)
	}
	var firstBalance, secondBalance, eventCount int64
	scan(t, f.db, `SELECT on_hand FROM inventory_balances WHERE company_id=$1 AND product_id=$2`, []any{f.company, f.product}, &firstBalance)
	scan(t, f.db, `SELECT on_hand FROM inventory_balances WHERE company_id=$1 AND product_id=$2`, []any{f.company, f.secondProduct}, &secondBalance)
	scan(t, f.db, `SELECT count(*) FROM inventory_outbound_events WHERE company_id=$1 AND batch_id=$2`, []any{f.company, batch2}, &eventCount)
	if firstBalance != 4 || secondBalance != 7 || eventCount != 0 {
		t.Fatalf("partial rollback balances=%d,%d events=%d", firstBalance, secondBalance, eventCount)
	}
}

func TestAmazonUsesCentralEcommerceOutboundEvent(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	exec(t, f.db, `INSERT INTO module_entitlements(company_id,module_key,enabled) VALUES($1,'amazon',true)`, f.company)
	var source, job, order, batch string
	scan(t, f.db, `INSERT INTO source_files(company_id,marketplace_key,storage_key,original_filename,content_type,size_bytes,sha256,uploaded_by) VALUES($1,'amazon',$2,'amazon-outbound.pdf','application/pdf',1,$3,$4) RETURNING id`, []any{f.company, "amazon-outbound-" + f.company, "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", f.user}, &source)
	scan(t, f.db, `INSERT INTO processing_jobs(company_id,source_file_id,marketplace_key,status,parser_version,total_pages,processed_pages) VALUES($1,$2,'amazon','processed','amazon-associated-v3',2,2) RETURNING id`, []any{f.company, source}, &job)
	scan(t, f.db, `INSERT INTO marketplace_orders(company_id,marketplace_key,source_file_id,processing_job_id,source_page,marketplace_order_id,status,parser_version) VALUES($1,'amazon',$2,$3,1,$4,'resolved','amazon-associated-v3') RETURNING id`, []any{f.company, source, job, "AMAZON-OUT-" + f.company}, &order)
	exec(t, f.db, `INSERT INTO marketplace_order_items(company_id,order_id,product_id,quantity,quantity_source,resolution_status) VALUES($1,$2,$3,4,'extracted','resolved')`, f.company, order, f.product)
	scan(t, f.db, `INSERT INTO batches(company_id,marketplace_key,status,created_by,idempotency_key,request_hash,ready_at) VALUES($1,'amazon','ready',$2,$3,$4,now()) RETURNING id`, []any{f.company, f.user, "amazon-outbound-batch-" + f.company, "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"}, &batch)
	exec(t, f.db, `INSERT INTO batch_members(company_id,batch_id,marketplace_order_id,position) VALUES($1,$2,$3,1)`, f.company, batch, order)
	if _, _, err := f.service.StockIn(ctx, f.principal, CommandInput{ProductID: f.product, Quantity: 9, Reason: "Amazon dispatch stock", IdempotencyKey: "amazon-dispatch-stock"}); err != nil {
		t.Fatal(err)
	}
	items, replay, err := f.service.ConfirmEcommerceOutbound(ctx, f.principal, batch, OutboundInput{IdempotencyKey: "amazon-dispatch"})
	if err != nil || replay || len(items) != 1 || items[0].QuantityDelta != -4 || items[0].ResultingBalance != 5 || items[0].ReferenceID == nil || *items[0].ReferenceID != batch {
		t.Fatalf("Amazon outbound=%#v replay=%v err=%v", items, replay, err)
	}
	items, replay, err = f.service.ConfirmEcommerceOutbound(ctx, f.principal, batch, OutboundInput{IdempotencyKey: "amazon-dispatch"})
	if err != nil || !replay || len(items) != 1 {
		t.Fatalf("Amazon replay=%#v replay=%v err=%v", items, replay, err)
	}
	var events, transactions, audits int
	scan(t, f.db, `SELECT count(*) FROM inventory_outbound_events WHERE company_id=$1 AND batch_id=$2`, []any{f.company, batch}, &events)
	scan(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1 AND transaction_type='ecommerce_out' AND reference_type='batch' AND reference_id=$2`, []any{f.company, batch}, &transactions)
	scan(t, f.db, `SELECT count(*) FROM audit_logs WHERE company_id=$1 AND action='inventory.ecommerce_out' AND target_id=$2`, []any{f.company, batch}, &audits)
	if events != 1 || transactions != 1 || audits != 1 {
		t.Fatalf("Amazon trace events=%d transactions=%d audits=%d", events, transactions, audits)
	}

	var secondOrder, secondBatch string
	scan(t, f.db, `INSERT INTO marketplace_orders(company_id,marketplace_key,source_file_id,processing_job_id,source_page,marketplace_order_id,status,parser_version) VALUES($1,'amazon',$2,$3,3,$4,'resolved','amazon-associated-v3') RETURNING id`, []any{f.company, source, job, "AMAZON-OUT-2-" + f.company}, &secondOrder)
	exec(t, f.db, `INSERT INTO marketplace_order_items(company_id,order_id,product_id,quantity,quantity_source,resolution_status) VALUES($1,$2,$3,6,'extracted','resolved')`, f.company, secondOrder, f.product)
	scan(t, f.db, `INSERT INTO batches(company_id,marketplace_key,status,created_by,idempotency_key,request_hash,ready_at) VALUES($1,'amazon','ready',$2,$3,$4,now()) RETURNING id`, []any{f.company, f.user, "amazon-outbound-short-" + f.company, "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"}, &secondBatch)
	exec(t, f.db, `INSERT INTO batch_members(company_id,batch_id,marketplace_order_id,position) VALUES($1,$2,$3,1)`, f.company, secondBatch, secondOrder)
	if _, _, err = f.service.ConfirmEcommerceOutbound(ctx, f.principal, secondBatch, OutboundInput{IdempotencyKey: "amazon-insufficient"}); !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("Amazon insufficient=%v", err)
	}
	var balance, failedEvents int64
	scan(t, f.db, `SELECT on_hand FROM inventory_balances WHERE company_id=$1 AND product_id=$2`, []any{f.company, f.product}, &balance)
	scan(t, f.db, `SELECT count(*) FROM inventory_outbound_events WHERE company_id=$1 AND batch_id=$2`, []any{f.company, secondBatch}, &failedEvents)
	if balance != 5 || failedEvents != 0 {
		t.Fatalf("Amazon atomic rollback balance=%d events=%d", balance, failedEvents)
	}
}

type execer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}
type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func exec(t *testing.T, db execer, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("exec: %v", err)
	}
}
func scan(t *testing.T, db queryer, query string, args []any, dest ...any) {
	t.Helper()
	if err := db.QueryRow(context.Background(), query, args...).Scan(dest...); err != nil {
		t.Fatalf("scan: %v", err)
	}
}
func stringPtr(value string) *string { return &value }
