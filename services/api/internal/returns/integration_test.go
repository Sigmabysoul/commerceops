// This file contains focused regression tests for the behavior owned by this package in the returns package.
package returns

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/inventory"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	db                                           *pgxpool.Pool
	service                                      *Service
	principal                                    auth.Principal
	company, otherCompany, user, role            string
	product, secondProduct, flipOrder, flipItem  string
	amazonOrder, amazonItem, concurrentOrder     string
	amazonSecondItem, concurrentItem, otherOrder string
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
	mustScan(t, db, `INSERT INTO companies(name) VALUES($1) RETURNING id`, []any{"Returns A " + suffix}, &f.company)
	mustScan(t, db, `INSERT INTO companies(name) VALUES($1) RETURNING id`, []any{"Returns B " + suffix}, &f.otherCompany)
	mustScan(t, db, `INSERT INTO users(email,password_hash) VALUES($1,'test') RETURNING id`, []any{"returns-" + suffix + "@example.test"}, &f.user)
	mustExec(t, db, `INSERT INTO company_users(company_id,user_id) VALUES($1,$3),($2,$3)`, f.company, f.otherCompany, f.user)
	mustScan(t, db, `INSERT INTO roles(company_id,name) VALUES($1,'Returns Operator') RETURNING id`, []any{f.company}, &f.role)
	mustExec(t, db, `INSERT INTO role_permissions(company_id,role_id,permission_key) VALUES($1,$2,'returns.view'),($1,$2,'returns.manage'),($1,$2,'returns.restock'),($1,$2,'labels.process'),($1,$2,'inventory.view'),($1,$2,'inventory.stock_in'),($1,$2,'inventory.dispatch'),($1,$2,'reports.view')`, f.company, f.role)
	mustExec(t, db, `INSERT INTO company_user_roles(company_id,user_id,role_id) VALUES($1,$2,$3)`, f.company, f.user, f.role)
	mustExec(t, db, `INSERT INTO module_entitlements(company_id,module_key,enabled) VALUES($1,'returns',true)`, f.company)
	mustExec(t, db, `INSERT INTO module_entitlements(company_id,module_key,enabled) VALUES($1,'inventory',true),($1,'flipkart',true),($1,'amazon',true),($1,'meesho',true)`, f.company)
	mustScan(t, db, `INSERT INTO products(company_id,internal_code,name) VALUES($1,'RET-1','Return product') RETURNING id`, []any{f.company}, &f.product)
	mustScan(t, db, `INSERT INTO products(company_id,internal_code,name) VALUES($1,'RET-2','Second return product') RETURNING id`, []any{f.company}, &f.secondProduct)
	otherProduct := ""
	mustScan(t, db, `INSERT INTO products(company_id,internal_code,name) VALUES($1,'RET-B','Other return product') RETURNING id`, []any{f.otherCompany}, &otherProduct)
	f.flipOrder, f.flipItem = createOrder(t, db, f.company, f.user, f.product, "flipkart", "FK-"+suffix, 4, "a")
	f.amazonOrder, f.amazonItem = createOrder(t, db, f.company, f.user, f.product, "amazon", "171-1234567-1234567", 5, "b")
	mustScan(t, db, `INSERT INTO marketplace_order_items(company_id,order_id,raw_sku,product_id,quantity,quantity_source,resolution_status) VALUES($1,$2,'RETURN-SKU-2',$3,2,'extracted','resolved') RETURNING id`, []any{f.company, f.amazonOrder, f.secondProduct}, &f.amazonSecondItem)
	f.concurrentOrder, f.concurrentItem = createOrder(t, db, f.company, f.user, f.product, "amazon", "171-7654321-7654321", 3, "c")
	f.otherOrder, _ = createOrder(t, db, f.otherCompany, f.user, otherProduct, "flipkart", "OTHER-"+suffix, 1, "d")
	var batch string
	mustScan(t, db, `INSERT INTO batches(company_id,marketplace_key,status,created_by,idempotency_key,request_hash,ready_at) VALUES($1,'amazon','ready',$2,$3,$4,now()) RETURNING id`, []any{f.company, f.user, "returns-outbound-" + suffix, fmt.Sprintf("%064x", 91)}, &batch)
	mustExec(t, db, `INSERT INTO batch_members(company_id,batch_id,marketplace_order_id,position) VALUES($1,$2,$3,1)`, f.company, batch, f.amazonOrder)
	mustExec(t, db, `INSERT INTO inventory_outbound_events(company_id,batch_id,actor_user_id,idempotency_key,request_hash) VALUES($1,$2,$3,$4,$5)`, f.company, batch, f.user, "returns-outbound-event-"+suffix, fmt.Sprintf("%064x", 92))
	f.principal = auth.Principal{CompanyID: f.company, UserID: f.user}
	authorizer := authorization.NewService(db)
	f.service = NewService(db, authorizer, inventory.NewService(db, authorizer))
	t.Cleanup(db.Close)
	return f
}

func TestCancellationClassificationIdempotencyAndIsolation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	at := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)
	before, replay, err := f.service.CreateCancellation(ctx, f.principal, CreateCancellationInput{MarketplaceOrderID: f.flipOrder, Reason: "Buyer cancelled before dispatch", CancelledAt: at, IdempotencyKey: "cancel-before"})
	if err != nil || replay || before.OutboundState != "not_outbound" || before.Marketplace != "flipkart" {
		t.Fatalf("before=%#v replay=%v err=%v", before, replay, err)
	}
	replayed, replay, err := f.service.CreateCancellation(ctx, f.principal, CreateCancellationInput{MarketplaceOrderID: f.flipOrder, Reason: "Buyer cancelled before dispatch", CancelledAt: at, IdempotencyKey: "cancel-before"})
	if err != nil || !replay || replayed.ID != before.ID {
		t.Fatalf("replayed=%#v replay=%v err=%v", replayed, replay, err)
	}
	if _, _, err = f.service.CreateCancellation(ctx, f.principal, CreateCancellationInput{MarketplaceOrderID: f.flipOrder, Reason: "Changed payload", CancelledAt: at, IdempotencyKey: "cancel-before"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("idempotency conflict=%v", err)
	}
	after, _, err := f.service.CreateCancellation(ctx, f.principal, CreateCancellationInput{MarketplaceOrderID: f.amazonOrder, Reason: "Carrier cancellation after dispatch", CancelledAt: at, IdempotencyKey: "cancel-after"})
	if err != nil || after.OutboundState != "outbound_confirmed" || after.Marketplace != "amazon" {
		t.Fatalf("after=%#v err=%v", after, err)
	}
	if _, _, err = f.service.CreateCancellation(ctx, f.principal, CreateCancellationInput{MarketplaceOrderID: f.flipOrder, Reason: "Duplicate event", CancelledAt: at, IdempotencyKey: "cancel-duplicate"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate order=%v", err)
	}
	if _, _, err = f.service.CreateCancellation(ctx, f.principal, CreateCancellationInput{MarketplaceOrderID: f.otherOrder, Reason: "Cross tenant", CancelledAt: at, IdempotencyKey: "cancel-cross"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross tenant=%v", err)
	}
	items, err := f.service.ListCancellations(ctx, f.principal, "recorded", "amazon")
	if err != nil || len(items) != 1 || items[0].ID != after.ID {
		t.Fatalf("filtered=%#v err=%v", items, err)
	}
	var transactions, audits int
	mustScan(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, []any{f.company}, &transactions)
	mustScan(t, f.db, `SELECT count(*) FROM audit_logs WHERE company_id=$1 AND action='returns.cancellation_recorded'`, []any{f.company}, &audits)
	if transactions != 0 || audits != 2 {
		t.Fatalf("transactions=%d audits=%d", transactions, audits)
	}
}

func TestReturnIntakePartialReceiptAndInventoryNeutrality(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	notes := "Package expected from carrier"
	created, replay, err := f.service.CreateReturn(ctx, f.principal, CreateReturnInput{MarketplaceOrderID: f.amazonOrder, Reason: "Customer return", Notes: &notes, Items: []ExpectedItemInput{{MarketplaceOrderItemID: f.amazonItem, ExpectedQuantity: 3}}, IdempotencyKey: "return-create"})
	if err != nil || replay || created.Status != "expected" || len(created.Items) != 1 || created.Items[0].ProductID != f.product || len(created.Events) != 1 || created.Events[0].EventType != "created" {
		t.Fatalf("created=%#v replay=%v err=%v", created, replay, err)
	}
	replayed, replay, err := f.service.CreateReturn(ctx, f.principal, CreateReturnInput{MarketplaceOrderID: f.amazonOrder, Reason: "Customer return", Notes: &notes, Items: []ExpectedItemInput{{MarketplaceOrderItemID: f.amazonItem, ExpectedQuantity: 3}}, IdempotencyKey: "return-create"})
	if err != nil || !replay || replayed.ID != created.ID {
		t.Fatalf("replayed=%#v replay=%v err=%v", replayed, replay, err)
	}
	received, replay, err := f.service.ReceiveReturn(ctx, f.principal, created.ID, ReceiveReturnInput{Items: []ReceivedItemInput{{ReturnItemID: created.Items[0].ID, ReceivedQuantity: 2}}, IdempotencyKey: "return-receive"})
	if err != nil || replay || received.Status != "received" || received.Items[0].ReceivedQuantity == nil || *received.Items[0].ReceivedQuantity != 2 || received.Items[0].Disposition != "pending" || len(received.Events) != 2 || received.Events[1].EventType != "received" {
		t.Fatalf("received=%#v replay=%v err=%v", received, replay, err)
	}
	received, replay, err = f.service.ReceiveReturn(ctx, f.principal, created.ID, ReceiveReturnInput{Items: []ReceivedItemInput{{ReturnItemID: created.Items[0].ID, ReceivedQuantity: 2}}, IdempotencyKey: "return-receive"})
	if err != nil || !replay || received.Status != "received" {
		t.Fatalf("receive replay=%#v replay=%v err=%v", received, replay, err)
	}
	receivedCases, err := f.service.ListReturns(ctx, f.principal, "received", "amazon")
	if err != nil || len(receivedCases) != 1 || receivedCases[0].ID != created.ID {
		t.Fatalf("received list=%#v err=%v", receivedCases, err)
	}
	second, _, err := f.service.CreateReturn(ctx, f.principal, CreateReturnInput{MarketplaceOrderID: f.amazonOrder, Reason: "Remaining units", Items: []ExpectedItemInput{{MarketplaceOrderItemID: f.amazonItem, ExpectedQuantity: 2}}, IdempotencyKey: "return-second"})
	if err != nil || second.Items[0].ExpectedQuantity != 2 {
		t.Fatalf("second=%#v err=%v", second, err)
	}
	if _, _, err = f.service.CreateReturn(ctx, f.principal, CreateReturnInput{MarketplaceOrderID: f.amazonOrder, Reason: "Exceeds order", Items: []ExpectedItemInput{{MarketplaceOrderItemID: f.amazonItem, ExpectedQuantity: 1}}, IdempotencyKey: "return-excess"}); !errors.Is(err, ErrQuantity) {
		t.Fatalf("excess=%v", err)
	}
	over, _, err := f.service.CreateReturn(ctx, f.principal, CreateReturnInput{MarketplaceOrderID: f.flipOrder, Reason: "Over receipt test", Items: []ExpectedItemInput{{MarketplaceOrderItemID: f.flipItem, ExpectedQuantity: 2}}, IdempotencyKey: "return-over"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.service.ReceiveReturn(ctx, f.principal, over.ID, ReceiveReturnInput{Items: []ReceivedItemInput{{ReturnItemID: over.Items[0].ID, ReceivedQuantity: 3}}, IdempotencyKey: "receive-over"}); !errors.Is(err, ErrQuantity) {
		t.Fatalf("over receipt=%v", err)
	}
	if _, _, err = f.service.CreateReturn(ctx, f.principal, CreateReturnInput{MarketplaceOrderID: f.otherOrder, Reason: "Cross tenant", Items: []ExpectedItemInput{{MarketplaceOrderItemID: f.flipItem, ExpectedQuantity: 1}}, IdempotencyKey: "return-cross"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross tenant=%v", err)
	}
	var transactions, audits int
	mustScan(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, []any{f.company}, &transactions)
	mustScan(t, f.db, `SELECT count(*) FROM audit_logs WHERE company_id=$1 AND target_type='return_case'`, []any{f.company}, &audits)
	if transactions != 0 || audits != 4 {
		t.Fatalf("transactions=%d audits=%d", transactions, audits)
	}
	if _, err = f.db.Exec(ctx, `UPDATE return_events SET notes='rewrite' WHERE company_id=$1 AND return_case_id=$2`, f.company, created.ID); err == nil {
		t.Fatal("return event history update was allowed")
	}
	var otherRole string
	mustExec(t, f.db, `INSERT INTO module_entitlements(company_id,module_key,enabled) VALUES($1,'returns',true)`, f.otherCompany)
	mustScan(t, f.db, `INSERT INTO roles(company_id,name) VALUES($1,'Other Returns Viewer') RETURNING id`, []any{f.otherCompany}, &otherRole)
	mustExec(t, f.db, `INSERT INTO role_permissions(company_id,role_id,permission_key) VALUES($1,$2,'returns.view')`, f.otherCompany, otherRole)
	mustExec(t, f.db, `INSERT INTO company_user_roles(company_id,user_id,role_id) VALUES($1,$2,$3)`, f.otherCompany, f.user, otherRole)
	if _, err = f.service.GetReturn(ctx, auth.Principal{CompanyID: f.otherCompany, UserID: f.user}, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant read=%v", err)
	}
}

func TestMeeshoCancellationAndReturnCompatibility(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	suffix := fmt.Sprint(time.Now().UnixNano())
	cancelOrder, _ := createOrder(t, f.db, f.company, f.user, f.product, "meesho", "MEESHO-CANCEL-"+suffix, 2, "e")
	returnOrder, returnItem := createOrder(t, f.db, f.company, f.user, f.product, "meesho", "MEESHO-RETURN-"+suffix, 3, "f")

	cancellation, replay, err := f.service.CreateCancellation(ctx, f.principal, CreateCancellationInput{MarketplaceOrderID: cancelOrder, Reason: "Meesho buyer cancellation", CancelledAt: time.Now(), IdempotencyKey: "meesho-cancel-" + suffix})
	if err != nil || replay || cancellation.Marketplace != "meesho" || cancellation.OutboundState != "not_outbound" {
		t.Fatalf("cancellation=%#v replay=%v err=%v", cancellation, replay, err)
	}
	created, replay, err := f.service.CreateReturn(ctx, f.principal, CreateReturnInput{MarketplaceOrderID: returnOrder, Reason: "Meesho physical return", Items: []ExpectedItemInput{{MarketplaceOrderItemID: returnItem, ExpectedQuantity: 2}}, IdempotencyKey: "meesho-return-" + suffix})
	if err != nil || replay || created.Marketplace != "meesho" || created.Status != "expected" || len(created.Items) != 1 || created.Items[0].ExpectedQuantity != 2 {
		t.Fatalf("return=%#v replay=%v err=%v", created, replay, err)
	}
	received, replay, err := f.service.ReceiveReturn(ctx, f.principal, created.ID, ReceiveReturnInput{Items: []ReceivedItemInput{{ReturnItemID: created.Items[0].ID, ReceivedQuantity: 1}}, IdempotencyKey: "meesho-receive-" + suffix})
	if err != nil || replay || received.Status != "received" || received.Items[0].ReceivedQuantity == nil || *received.Items[0].ReceivedQuantity != 1 {
		t.Fatalf("received=%#v replay=%v err=%v", received, replay, err)
	}
	returns, err := f.service.ListReturns(ctx, f.principal, "received", "meesho")
	if err != nil || len(returns) != 1 || returns[0].ID != created.ID {
		t.Fatalf("returns=%#v err=%v", returns, err)
	}
	cancellations, err := f.service.ListCancellations(ctx, f.principal, "recorded", "meesho")
	if err != nil || len(cancellations) != 1 || cancellations[0].ID != cancellation.ID {
		t.Fatalf("cancellations=%#v err=%v", cancellations, err)
	}
	var inventoryTransactions int
	mustScan(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, []any{f.company}, &inventoryTransactions)
	if inventoryTransactions != 0 {
		t.Fatalf("Meesho cancellation/receipt changed inventory: %d transactions", inventoryTransactions)
	}
}

func TestSnapdealCancellationAndReturnCompatibility(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	suffix := fmt.Sprint(time.Now().UnixNano())
	cancelOrder, _ := createOrder(t, f.db, f.company, f.user, f.product, "snapdeal", "SNAP-CANCEL-"+suffix, 2, "a")
	returnOrder, returnItem := createOrder(t, f.db, f.company, f.user, f.product, "snapdeal", "SNAP-RETURN-"+suffix, 3, "b")
	c, replay, err := f.service.CreateCancellation(ctx, f.principal, CreateCancellationInput{MarketplaceOrderID: cancelOrder, Reason: "Snapdeal buyer cancellation", CancelledAt: time.Now(), IdempotencyKey: "snap-cancel-" + suffix})
	if err != nil || replay || c.Marketplace != "snapdeal" || c.OutboundState != "not_outbound" {
		t.Fatalf("cancellation=%#v replay=%v err=%v", c, replay, err)
	}
	created, replay, err := f.service.CreateReturn(ctx, f.principal, CreateReturnInput{MarketplaceOrderID: returnOrder, Reason: "Snapdeal physical return", Items: []ExpectedItemInput{{MarketplaceOrderItemID: returnItem, ExpectedQuantity: 2}}, IdempotencyKey: "snap-return-" + suffix})
	if err != nil || replay || created.Marketplace != "snapdeal" || created.Items[0].ExpectedQuantity != 2 {
		t.Fatalf("return=%#v replay=%v err=%v", created, replay, err)
	}
	received, replay, err := f.service.ReceiveReturn(ctx, f.principal, created.ID, ReceiveReturnInput{Items: []ReceivedItemInput{{ReturnItemID: created.Items[0].ID, ReceivedQuantity: 1}}, IdempotencyKey: "snap-receive-" + suffix})
	if err != nil || replay || received.Status != "received" {
		t.Fatalf("received=%#v replay=%v err=%v", received, replay, err)
	}
	items, err := f.service.ListReturns(ctx, f.principal, "received", "snapdeal")
	if err != nil || len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("returns=%#v err=%v", items, err)
	}
	var inventory int
	mustScan(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, []any{f.company}, &inventory)
	if inventory != 0 {
		t.Fatalf("return intake changed inventory=%d", inventory)
	}
}

func TestReturnAuthorizationAndConcurrentQuantityBound(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	type replayResult struct {
		replay bool
		err    error
	}
	replayStart := make(chan struct{})
	replayResults := make(chan replayResult, 2)
	var replayWG sync.WaitGroup
	for index := 0; index < 2; index++ {
		replayWG.Add(1)
		go func() {
			defer replayWG.Done()
			<-replayStart
			_, replay, err := f.service.CreateReturn(ctx, f.principal, CreateReturnInput{MarketplaceOrderID: f.flipOrder, Reason: "Concurrent exact retry", Items: []ExpectedItemInput{{MarketplaceOrderItemID: f.flipItem, ExpectedQuantity: 1}}, IdempotencyKey: "concurrent-exact-return"})
			replayResults <- replayResult{replay: replay, err: err}
		}()
	}
	close(replayStart)
	replayWG.Wait()
	close(replayResults)
	replayCount, createCount := 0, 0
	for result := range replayResults {
		if result.err != nil {
			t.Fatalf("concurrent replay error=%v", result.err)
		}
		if result.replay {
			replayCount++
		} else {
			createCount++
		}
	}
	if replayCount != 1 || createCount != 1 {
		t.Fatalf("concurrent exact create=%d replay=%d", createCount, replayCount)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for index := 0; index < 2; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			_, _, err := f.service.CreateReturn(ctx, f.principal, CreateReturnInput{MarketplaceOrderID: f.concurrentOrder, Reason: "Concurrent intake", Items: []ExpectedItemInput{{MarketplaceOrderItemID: f.concurrentItem, ExpectedQuantity: 2}}, IdempotencyKey: fmt.Sprintf("concurrent-return-%d", index)})
			results <- err
		}(index)
	}
	close(start)
	wg.Wait()
	close(results)
	var outcomes []string
	for err := range results {
		switch {
		case err == nil:
			outcomes = append(outcomes, "success")
		case errors.Is(err, ErrQuantity):
			outcomes = append(outcomes, "quantity")
		default:
			t.Fatalf("unexpected concurrent error=%v", err)
		}
	}
	sort.Strings(outcomes)
	if fmt.Sprint(outcomes) != "[quantity success]" {
		t.Fatalf("outcomes=%v", outcomes)
	}
	mustExec(t, f.db, `DELETE FROM role_permissions WHERE company_id=$1 AND role_id=$2 AND permission_key='returns.manage'`, f.company, f.role)
	if _, _, err := f.service.CreateCancellation(ctx, f.principal, CreateCancellationInput{MarketplaceOrderID: f.flipOrder, Reason: "Denied", CancelledAt: time.Now(), IdempotencyKey: "denied"}); !errors.Is(err, authorization.ErrPermissionDenied) {
		t.Fatalf("manage permission=%v", err)
	}
	if _, err := f.service.ListReturns(ctx, auth.Principal{CompanyID: f.otherCompany, UserID: f.user}, "", ""); !errors.Is(err, authorization.ErrModuleUnavailable) {
		t.Fatalf("module entitlement=%v", err)
	}
}

func TestReturnsMigrationUpDown(t *testing.T) {
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
	schema := "p8_migration_" + fmt.Sprint(time.Now().UnixNano())
	mustExec(t, tx, `CREATE SCHEMA `+schema)
	mustExec(t, tx, `SET LOCAL search_path TO `+schema+`,public`)
	root := filepath.Join("..", "..", "migrations")
	files, err := filepath.Glob(filepath.Join(root, "*.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	for _, file := range files {
		if filepath.Base(file) > "000014_returns_cancellations_foundation.up.sql" {
			break
		}
		data, readErr := os.ReadFile(file)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = tx.Exec(ctx, string(data)); err != nil {
			t.Fatalf("apply %s: %v", filepath.Base(file), err)
		}
	}
	var exists *string
	if err = tx.QueryRow(ctx, `SELECT to_regclass($1)`, schema+".return_cases").Scan(&exists); err != nil || exists == nil {
		t.Fatalf("up=%v err=%v", exists, err)
	}
	down, err := os.ReadFile(filepath.Join(root, "000014_returns_cancellations_foundation.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(down)); err != nil {
		t.Fatalf("down: %v", err)
	}
	exists = nil
	if err = tx.QueryRow(ctx, `SELECT to_regclass($1)`, schema+".return_cases").Scan(&exists); err != nil || exists != nil {
		t.Fatalf("down=%v err=%v", exists, err)
	}
}

func TestReturnDispositionInventoryMigrationUpDown(t *testing.T) {
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
	schema := "p8b_migration_" + fmt.Sprint(time.Now().UnixNano())
	mustExec(t, tx, `CREATE SCHEMA `+schema)
	mustExec(t, tx, `SET LOCAL search_path TO `+schema+`,public`)
	root := filepath.Join("..", "..", "migrations")
	files, err := filepath.Glob(filepath.Join(root, "*.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	for _, file := range files {
		if filepath.Base(file) > "000015_return_disposition_inventory.up.sql" {
			break
		}
		data, readErr := os.ReadFile(file)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = tx.Exec(ctx, string(data)); err != nil {
			t.Fatalf("apply %s: %v", filepath.Base(file), err)
		}
	}
	var columnCount int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema=$1 AND table_name='return_items' AND column_name IN ('restocked_quantity','corrected_quantity')`, schema).Scan(&columnCount); err != nil || columnCount != 2 {
		t.Fatalf("columns=%d err=%v", columnCount, err)
	}
	var exists *string
	if err = tx.QueryRow(ctx, `SELECT to_regclass($1)`, schema+".inventory_transactions_return_restock_source_idx").Scan(&exists); err != nil || exists == nil {
		t.Fatalf("index=%v err=%v", exists, err)
	}
	down, err := os.ReadFile(filepath.Join(root, "000015_return_disposition_inventory.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(down)); err != nil {
		t.Fatalf("down: %v", err)
	}
	columnCount = 0
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema=$1 AND table_name='return_items' AND column_name IN ('restocked_quantity','corrected_quantity')`, schema).Scan(&columnCount); err != nil || columnCount != 0 {
		t.Fatalf("down columns=%d err=%v", columnCount, err)
	}
}

func TestReturnCancellationClosureMigrationUpDown(t *testing.T) {
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
	schema := "p8c_migration_" + fmt.Sprint(time.Now().UnixNano())
	mustExec(t, tx, `CREATE SCHEMA `+schema)
	mustExec(t, tx, `SET LOCAL search_path TO `+schema+`,public`)
	root := filepath.Join("..", "..", "migrations")
	files, err := filepath.Glob(filepath.Join(root, "*.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	for _, file := range files {
		data, readErr := os.ReadFile(file)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = tx.Exec(ctx, string(data)); err != nil {
			t.Fatalf("apply %s: %v", filepath.Base(file), err)
		}
	}
	var exists *string
	if err = tx.QueryRow(ctx, `SELECT to_regclass($1)`, schema+".cancellation_events").Scan(&exists); err != nil || exists == nil {
		t.Fatalf("events=%v err=%v", exists, err)
	}
	if err = tx.QueryRow(ctx, `SELECT to_regclass($1)`, schema+".return_events_company_type_created_idx").Scan(&exists); err != nil || exists == nil {
		t.Fatalf("reporting index=%v err=%v", exists, err)
	}
	var columnCount int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema=$1 AND table_name IN ('return_cases','cancellations') AND column_name IN ('closed_by','closed_at')`, schema).Scan(&columnCount); err != nil || columnCount != 4 {
		t.Fatalf("closure columns=%d err=%v", columnCount, err)
	}
	down, err := os.ReadFile(filepath.Join(root, "000016_return_cancellation_closure.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(down)); err != nil {
		t.Fatalf("down: %v", err)
	}
	exists = nil
	if err = tx.QueryRow(ctx, `SELECT to_regclass($1)`, schema+".cancellation_events").Scan(&exists); err != nil || exists != nil {
		t.Fatalf("down events=%v err=%v", exists, err)
	}
}

func createOrder(t *testing.T, db *pgxpool.Pool, company, user, product, marketplace, externalID string, quantity int64, hashByte string) (string, string) {
	t.Helper()
	suffix := fmt.Sprint(time.Now().UnixNano())
	var source, job, order, item string
	mustScan(t, db, `INSERT INTO source_files(company_id,marketplace_key,storage_key,original_filename,content_type,size_bytes,sha256,uploaded_by) VALUES($1,$2,$3,$4,'application/pdf',1,$5,$6) RETURNING id`, []any{company, marketplace, marketplace + "/returns/" + suffix, marketplace + ".pdf", strings.Repeat(hashByte, 64), user}, &source)
	mustScan(t, db, `INSERT INTO processing_jobs(company_id,source_file_id,marketplace_key,status,parser_version,total_pages,processed_pages) VALUES($1,$2,$3,'processed','returns-test',1,1) RETURNING id`, []any{company, source, marketplace}, &job)
	mustScan(t, db, `INSERT INTO marketplace_orders(company_id,marketplace_key,source_file_id,processing_job_id,source_page,marketplace_order_id,status,parser_version) VALUES($1,$2,$3,$4,1,$5,'resolved','returns-test') RETURNING id`, []any{company, marketplace, source, job, externalID}, &order)
	mustScan(t, db, `INSERT INTO marketplace_order_items(company_id,order_id,raw_sku,product_id,quantity,quantity_source,resolution_status) VALUES($1,$2,'RETURN-SKU',$3,$4,'extracted','resolved') RETURNING id`, []any{company, order, product, quantity}, &item)
	return order, item
}

type commandExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}
type rowQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func mustExec(t *testing.T, db commandExecer, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(context.Background(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func mustScan(t *testing.T, db rowQueryer, query string, args []any, destinations ...any) {
	t.Helper()
	if err := db.QueryRow(context.Background(), query, args...).Scan(destinations...); err != nil {
		t.Fatal(err)
	}
}
