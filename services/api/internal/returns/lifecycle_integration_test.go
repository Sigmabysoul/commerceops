package returns

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/batch"
	"github.com/commerceops/commerceops/services/api/internal/inventory"
	"github.com/commerceops/commerceops/services/api/internal/reporting"
)

func TestInspectionRestockAndCompensatingCorrection(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	created, _, err := f.service.CreateReturn(ctx, f.principal, CreateReturnInput{
		MarketplaceOrderID: f.amazonOrder,
		Reason:             "Mixed-condition physical return",
		Items: []ExpectedItemInput{
			{MarketplaceOrderItemID: f.amazonItem, ExpectedQuantity: 4},
			{MarketplaceOrderItemID: f.amazonSecondItem, ExpectedQuantity: 2},
		},
		IdempotencyKey: "lifecycle-create",
	})
	if err != nil {
		t.Fatal(err)
	}
	receivedInputs := make([]ReceivedItemInput, 0, 2)
	for _, item := range created.Items {
		quantity := int64(2)
		if item.ProductID == f.product {
			quantity = 3
		}
		receivedInputs = append(receivedInputs, ReceivedItemInput{ReturnItemID: item.ID, ReceivedQuantity: quantity})
	}
	received, _, err := f.service.ReceiveReturn(ctx, f.principal, created.ID, ReceiveReturnInput{
		Items:          receivedInputs,
		IdempotencyKey: "lifecycle-receive",
	})
	if err != nil || received.Status != "received" {
		t.Fatalf("received=%#v err=%v", received, err)
	}
	dispositions := make([]DispositionItemInput, 0, 2)
	for _, item := range received.Items {
		disposition := "damaged"
		if item.ProductID == f.product {
			disposition = "restockable"
		}
		dispositions = append(dispositions, DispositionItemInput{ReturnItemID: item.ID, Disposition: disposition})
	}
	inspected, replay, err := f.service.InspectReturn(ctx, f.principal, created.ID, InspectReturnInput{Items: dispositions, IdempotencyKey: "lifecycle-inspect"})
	if err != nil || replay || inspected.Status != "inspected" || len(inspected.Events) != 3 {
		t.Fatalf("inspected=%#v replay=%v err=%v", inspected, replay, err)
	}
	inspected, replay, err = f.service.InspectReturn(ctx, f.principal, created.ID, InspectReturnInput{Items: dispositions, IdempotencyKey: "lifecycle-inspect"})
	if err != nil || !replay || inspected.Status != "inspected" {
		t.Fatalf("inspection replay=%#v replay=%v err=%v", inspected, replay, err)
	}
	restocked, replay, err := f.service.RestockReturn(ctx, f.principal, created.ID, RestockReturnInput{IdempotencyKey: "lifecycle-restock"})
	if err != nil || replay || restocked.Status != "restocked" || len(restocked.InventoryImpact) != 1 || restocked.InventoryImpact[0].TransactionType != "return_restock" || restocked.InventoryImpact[0].QuantityDelta != 3 {
		t.Fatalf("restocked=%#v replay=%v err=%v", restocked, replay, err)
	}
	for _, item := range restocked.Items {
		if item.ProductID == f.product && item.RestockedQuantity != 3 {
			t.Fatalf("restockable item=%#v", item)
		}
		if item.ProductID == f.secondProduct && item.RestockedQuantity != 0 {
			t.Fatalf("damaged item was restocked=%#v", item)
		}
	}
	restocked, replay, err = f.service.RestockReturn(ctx, f.principal, created.ID, RestockReturnInput{IdempotencyKey: "lifecycle-restock"})
	if err != nil || !replay || len(restocked.InventoryImpact) != 1 {
		t.Fatalf("restock replay=%#v replay=%v err=%v", restocked, replay, err)
	}
	returnItemID := ""
	for _, item := range restocked.Items {
		if item.ProductID == f.product {
			returnItemID = item.ID
		}
	}
	corrected, replay, err := f.service.CorrectRestock(ctx, f.principal, created.ID, CorrectRestockInput{Items: []CorrectionItemInput{{ReturnItemID: returnItemID, Quantity: 1}}, Reason: "One unit was not actually sellable", IdempotencyKey: "lifecycle-correct"})
	if err != nil || replay || corrected.Status != "restock_corrected" || len(corrected.InventoryImpact) != 2 || corrected.InventoryImpact[1].TransactionType != "correction" || corrected.InventoryImpact[1].QuantityDelta != -1 {
		t.Fatalf("corrected=%#v replay=%v err=%v", corrected, replay, err)
	}
	corrected, replay, err = f.service.CorrectRestock(ctx, f.principal, created.ID, CorrectRestockInput{Items: []CorrectionItemInput{{ReturnItemID: returnItemID, Quantity: 1}}, Reason: "One unit was not actually sellable", IdempotencyKey: "lifecycle-correct"})
	if err != nil || !replay || len(corrected.InventoryImpact) != 2 {
		t.Fatalf("correction replay=%#v replay=%v err=%v", corrected, replay, err)
	}
	reportingService := reporting.NewService(f.db, authorization.NewService(f.db))
	from, to := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	report, err := reportingService.Dashboard(ctx, f.principal, reporting.Filter{From: from, To: to, Marketplace: "amazon", Limit: 50})
	if err != nil || report.Inventory == nil || report.Inventory.ReturnRestock != 2 || report.Inventory.Adjustments != 0 || report.Inventory.NetMovement != 2 {
		t.Fatalf("Amazon return movement report=%#v err=%v", report.Inventory, err)
	}
	report, err = reportingService.Dashboard(ctx, f.principal, reporting.Filter{From: from, To: to, Marketplace: "flipkart", Limit: 50})
	if err != nil || report.Inventory == nil || report.Inventory.ReturnRestock != 0 {
		t.Fatalf("Flipkart return movement report=%#v err=%v", report.Inventory, err)
	}
	if _, _, err = f.service.CorrectRestock(ctx, f.principal, created.ID, CorrectRestockInput{Items: []CorrectionItemInput{{ReturnItemID: returnItemID, Quantity: 3}}, Reason: "Too many", IdempotencyKey: "lifecycle-over-correct"}); !errors.Is(err, ErrQuantity) {
		t.Fatalf("over correction=%v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for index := 0; index < 2; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			_, _, runErr := f.service.CorrectRestock(ctx, f.principal, created.ID, CorrectRestockInput{Items: []CorrectionItemInput{{ReturnItemID: returnItemID, Quantity: 2}}, Reason: "Concurrent correction", IdempotencyKey: fmt.Sprintf("lifecycle-concurrent-correct-%d", index)})
			results <- runErr
		}(index)
	}
	close(start)
	wg.Wait()
	close(results)
	success, bounded := 0, 0
	for result := range results {
		if result == nil {
			success++
		} else if errors.Is(result, ErrQuantity) {
			bounded++
		} else {
			t.Fatalf("concurrent correction=%v", result)
		}
	}
	if success != 1 || bounded != 1 {
		t.Fatalf("concurrent correction success=%d bounded=%d", success, bounded)
	}
	var balance, ledgerTotal, transactionCount int64
	mustScan(t, f.db, `SELECT on_hand FROM inventory_balances WHERE company_id=$1 AND product_id=$2`, []any{f.company, f.product}, &balance)
	mustScan(t, f.db, `SELECT COALESCE(sum(quantity_delta),0),count(*) FROM inventory_transactions WHERE company_id=$1 AND product_id=$2 AND (transaction_type='return_restock' OR (transaction_type='correction' AND reference_type='return_restock_correction'))`, []any{f.company, f.product}, &ledgerTotal, &transactionCount)
	if balance != 0 || ledgerTotal != 0 || transactionCount != 3 {
		t.Fatalf("reconciliation balance=%d ledger=%d transactions=%d", balance, ledgerTotal, transactionCount)
	}
}

func TestNonRestockableAndMissingReturnsRemainInventoryNeutral(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	damaged, _, err := f.service.CreateReturn(ctx, f.principal, CreateReturnInput{MarketplaceOrderID: f.flipOrder, Reason: "Damaged return", Items: []ExpectedItemInput{{MarketplaceOrderItemID: f.flipItem, ExpectedQuantity: 2}}, IdempotencyKey: "damaged-create"})
	if err != nil {
		t.Fatal(err)
	}
	damaged, _, err = f.service.ReceiveReturn(ctx, f.principal, damaged.ID, ReceiveReturnInput{Items: []ReceivedItemInput{{ReturnItemID: damaged.Items[0].ID, ReceivedQuantity: 2}}, IdempotencyKey: "damaged-receive"})
	if err != nil {
		t.Fatal(err)
	}
	damaged, _, err = f.service.InspectReturn(ctx, f.principal, damaged.ID, InspectReturnInput{Items: []DispositionItemInput{{ReturnItemID: damaged.Items[0].ID, Disposition: "damaged"}}, IdempotencyKey: "damaged-inspect"})
	if err != nil || damaged.Status != "damaged" {
		t.Fatalf("damaged=%#v err=%v", damaged, err)
	}
	if _, _, err = f.service.RestockReturn(ctx, f.principal, damaged.ID, RestockReturnInput{IdempotencyKey: "damaged-restock"}); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("damaged restock=%v", err)
	}
	missing, _, err := f.service.CreateReturn(ctx, f.principal, CreateReturnInput{MarketplaceOrderID: f.flipOrder, Reason: "Missing return", Items: []ExpectedItemInput{{MarketplaceOrderItemID: f.flipItem, ExpectedQuantity: 2}}, IdempotencyKey: "missing-create"})
	if err != nil {
		t.Fatal(err)
	}
	missing, _, err = f.service.ReceiveReturn(ctx, f.principal, missing.ID, ReceiveReturnInput{Items: []ReceivedItemInput{{ReturnItemID: missing.Items[0].ID, ReceivedQuantity: 0}}, IdempotencyKey: "missing-receive"})
	if err != nil {
		t.Fatal(err)
	}
	missing, _, err = f.service.InspectReturn(ctx, f.principal, missing.ID, InspectReturnInput{Items: []DispositionItemInput{{ReturnItemID: missing.Items[0].ID, Disposition: "missing"}}, IdempotencyKey: "missing-inspect"})
	if err != nil || missing.Status != "rejected" {
		t.Fatalf("missing=%#v err=%v", missing, err)
	}
	var transactions int
	mustScan(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, []any{f.company}, &transactions)
	if transactions != 0 {
		t.Fatalf("non-restockable inventory transactions=%d", transactions)
	}
}

func TestRestockAuthorizationAndInventoryEntitlement(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	item, _, err := f.service.CreateReturn(ctx, f.principal, CreateReturnInput{MarketplaceOrderID: f.concurrentOrder, Reason: "Authorization return", Items: []ExpectedItemInput{{MarketplaceOrderItemID: f.concurrentItem, ExpectedQuantity: 1}}, IdempotencyKey: "auth-create"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.ReceiveReturn(ctx, f.principal, item.ID, ReceiveReturnInput{Items: []ReceivedItemInput{{ReturnItemID: item.Items[0].ID, ReceivedQuantity: 1}}, IdempotencyKey: "auth-receive"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.InspectReturn(ctx, f.principal, item.ID, InspectReturnInput{Items: []DispositionItemInput{{ReturnItemID: item.Items[0].ID, Disposition: "restockable"}}, IdempotencyKey: "auth-inspect"})
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, f.db, `DELETE FROM role_permissions WHERE company_id=$1 AND role_id=$2 AND permission_key='returns.restock'`, f.company, f.role)
	if _, _, err = f.service.RestockReturn(ctx, f.principal, item.ID, RestockReturnInput{IdempotencyKey: "auth-restock-denied"}); !errors.Is(err, authorization.ErrPermissionDenied) {
		t.Fatalf("restock permission=%v", err)
	}
	mustExec(t, f.db, `INSERT INTO role_permissions(company_id,role_id,permission_key) VALUES($1,$2,'returns.restock')`, f.company, f.role)
	mustExec(t, f.db, `UPDATE module_entitlements SET enabled=false WHERE company_id=$1 AND module_key='inventory'`, f.company)
	if _, _, err = f.service.RestockReturn(ctx, f.principal, item.ID, RestockReturnInput{IdempotencyKey: "auth-restock-module"}); !errors.Is(err, authorization.ErrModuleUnavailable) {
		t.Fatalf("inventory entitlement=%v", err)
	}
	var transactions int
	mustScan(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, []any{f.company}, &transactions)
	if transactions != 0 {
		t.Fatalf("unauthorized transactions=%d", transactions)
	}
}

func TestConcurrentRestockIsAnExactReplay(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	item, _, err := f.service.CreateReturn(ctx, f.principal, CreateReturnInput{MarketplaceOrderID: f.concurrentOrder, Reason: "Concurrent restock", Items: []ExpectedItemInput{{MarketplaceOrderItemID: f.concurrentItem, ExpectedQuantity: 1}}, IdempotencyKey: "concurrent-restock-create"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.ReceiveReturn(ctx, f.principal, item.ID, ReceiveReturnInput{Items: []ReceivedItemInput{{ReturnItemID: item.Items[0].ID, ReceivedQuantity: 1}}, IdempotencyKey: "concurrent-restock-receive"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.InspectReturn(ctx, f.principal, item.ID, InspectReturnInput{Items: []DispositionItemInput{{ReturnItemID: item.Items[0].ID, Disposition: "restockable"}}, IdempotencyKey: "concurrent-restock-inspect"})
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		replay bool
		err    error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for index := 0; index < 2; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, replay, runErr := f.service.RestockReturn(ctx, f.principal, item.ID, RestockReturnInput{IdempotencyKey: "concurrent-restock"})
			results <- result{replay: replay, err: runErr}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	created, replayed := 0, 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent restock=%v", result.err)
		}
		if result.replay {
			replayed++
		} else {
			created++
		}
	}
	var transactions, balance int64
	mustScan(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1 AND transaction_type='return_restock' AND reference_type='return_case' AND reference_id=$2`, []any{f.company, item.ID}, &transactions)
	mustScan(t, f.db, `SELECT on_hand FROM inventory_balances WHERE company_id=$1 AND product_id=$2`, []any{f.company, f.product}, &balance)
	if created != 1 || replayed != 1 || transactions != 1 || balance != 1 {
		t.Fatalf("created=%d replayed=%d transactions=%d balance=%d", created, replayed, transactions, balance)
	}
}

func TestCancellationBlocksBatchAndSerializesWithOutbound(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	authorizer := authorization.NewService(f.db)
	batchService := batch.NewService(f.db, authorizer)
	inventoryService := inventory.NewService(f.db, authorizer)
	_, _, err := f.service.CreateCancellation(ctx, f.principal, CreateCancellationInput{MarketplaceOrderID: f.flipOrder, Reason: "Cancelled before batching", CancelledAt: time.Now(), IdempotencyKey: "batch-cancelled-order"})
	if err != nil {
		t.Fatal(err)
	}
	eligible, err := batchService.EligibleOrders(ctx, f.principal, "flipkart")
	if err != nil {
		t.Fatal(err)
	}
	for _, order := range eligible {
		if order.OrderID == f.flipOrder {
			t.Fatal("cancelled order remained batch eligible")
		}
	}
	if _, _, err = batchService.Create(ctx, f.principal, batch.CreateInput{MarketplaceKey: "flipkart", OrderIDs: []string{f.flipOrder}, IdempotencyKey: "cancelled-batch-create"}); !errors.Is(err, batch.ErrIneligible) {
		t.Fatalf("cancelled batch create=%v", err)
	}
	draft, _, err := batchService.Create(ctx, f.principal, batch.CreateInput{MarketplaceKey: "amazon", OrderIDs: []string{f.concurrentOrder}, IdempotencyKey: "draft-before-cancel"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.service.CreateCancellation(ctx, f.principal, CreateCancellationInput{MarketplaceOrderID: f.concurrentOrder, Reason: "Cancelled after draft", CancelledAt: time.Now(), IdempotencyKey: "draft-order-cancel"}); err != nil {
		t.Fatal(err)
	}
	if _, err = batchService.Ready(ctx, f.principal, draft.ID); !errors.Is(err, batch.ErrIneligible) {
		t.Fatalf("cancelled draft readiness=%v", err)
	}

	blockedOrder, _ := createOrder(t, f.db, f.company, f.user, f.product, "flipkart", "BLOCKED-OUTBOUND-"+fmt.Sprint(time.Now().UnixNano()), 1, "e")
	blockedBatch := readyBatch(t, f, blockedOrder, "blocked-outbound")
	if _, _, err = f.service.CreateCancellation(ctx, f.principal, CreateCancellationInput{MarketplaceOrderID: blockedOrder, Reason: "Cancelled before outbound", CancelledAt: time.Now(), IdempotencyKey: "blocked-outbound-cancel"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = inventoryService.ConfirmEcommerceOutbound(ctx, f.principal, blockedBatch, inventory.OutboundInput{IdempotencyKey: "blocked-outbound-confirm"}); !errors.Is(err, inventory.ErrCancelledOrder) {
		t.Fatalf("cancelled outbound=%v", err)
	}

	raceOrder, _ := createOrder(t, f.db, f.company, f.user, f.product, "flipkart", "RACE-OUTBOUND-"+fmt.Sprint(time.Now().UnixNano()), 2, "f")
	raceBatch := readyBatch(t, f, raceOrder, "race-outbound")
	if _, _, err = inventoryService.StockIn(ctx, f.principal, inventory.CommandInput{ProductID: f.product, Quantity: 10, Reason: "Race stock", IdempotencyKey: "race-stock"}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	type cancellationResult struct {
		item Cancellation
		err  error
	}
	cancelResult := make(chan cancellationResult, 1)
	outboundResult := make(chan error, 1)
	go func() {
		<-start
		item, _, runErr := f.service.CreateCancellation(ctx, f.principal, CreateCancellationInput{MarketplaceOrderID: raceOrder, Reason: "Concurrent cancellation", CancelledAt: time.Now(), IdempotencyKey: "race-cancel"})
		cancelResult <- cancellationResult{item: item, err: runErr}
	}()
	go func() {
		<-start
		_, _, runErr := inventoryService.ConfirmEcommerceOutbound(ctx, f.principal, raceBatch, inventory.OutboundInput{IdempotencyKey: "race-outbound-confirm"})
		outboundResult <- runErr
	}()
	close(start)
	cancelled := <-cancelResult
	outboundErr := <-outboundResult
	if cancelled.err != nil {
		t.Fatal(cancelled.err)
	}
	var balance, eventCount int64
	mustScan(t, f.db, `SELECT on_hand FROM inventory_balances WHERE company_id=$1 AND product_id=$2`, []any{f.company, f.product}, &balance)
	mustScan(t, f.db, `SELECT count(*) FROM inventory_outbound_events WHERE company_id=$1 AND batch_id=$2`, []any{f.company, raceBatch}, &eventCount)
	switch {
	case outboundErr == nil:
		if cancelled.item.OutboundState != "outbound_confirmed" || balance != 8 || eventCount != 1 {
			t.Fatalf("outbound won cancellation=%#v balance=%d events=%d", cancelled.item, balance, eventCount)
		}
		if _, replay, replayErr := inventoryService.ConfirmEcommerceOutbound(ctx, f.principal, raceBatch, inventory.OutboundInput{IdempotencyKey: "race-outbound-confirm"}); replayErr != nil || !replay {
			t.Fatalf("post-cancellation outbound replay=%v replay=%v", replayErr, replay)
		}
	case errors.Is(outboundErr, inventory.ErrCancelledOrder):
		if cancelled.item.OutboundState != "not_outbound" || balance != 10 || eventCount != 0 {
			t.Fatalf("cancellation won cancellation=%#v balance=%d events=%d", cancelled.item, balance, eventCount)
		}
	default:
		t.Fatalf("race outbound=%v", outboundErr)
	}
}

func readyBatch(t *testing.T, f *fixture, orderID, key string) string {
	t.Helper()
	var batchID string
	mustScan(t, f.db, `INSERT INTO batches(company_id,marketplace_key,status,created_by,idempotency_key,request_hash,ready_at) VALUES($1,'flipkart','ready',$2,$3,$4,now()) RETURNING id`, []any{f.company, f.user, key + "-" + f.company, fmt.Sprintf("%064x", time.Now().UnixNano())}, &batchID)
	mustExec(t, f.db, `INSERT INTO batch_members(company_id,batch_id,marketplace_order_id,position) VALUES($1,$2,$3,1)`, f.company, batchID, orderID)
	return batchID
}
