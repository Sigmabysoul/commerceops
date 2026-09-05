// This file contains PostgreSQL-backed tests for cross-layer behavior, tenant isolation, and domain invariants in the returns package.
package returns

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/reporting"
)

func TestReturnAndCancellationClosureLifecycle(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	cancellation, _, err := f.service.CreateCancellation(ctx, f.principal, CreateCancellationInput{
		MarketplaceOrderID: f.flipOrder,
		Reason:             "Cancelled before dispatch",
		CancelledAt:        time.Now(),
		IdempotencyKey:     "closure-cancellation-create",
	})
	if err != nil {
		t.Fatal(err)
	}
	closedCancellation, replay, err := f.service.CloseCancellation(ctx, f.principal, cancellation.ID, CloseInput{IdempotencyKey: "closure-cancellation-close"})
	if err != nil || replay || closedCancellation.Status != "closed" || closedCancellation.ClosedBy == nil || closedCancellation.ClosedAt == nil || len(closedCancellation.Events) != 1 || closedCancellation.Events[0].EventType != "closed" {
		t.Fatalf("closed cancellation=%#v replay=%v err=%v", closedCancellation, replay, err)
	}
	closedCancellation, replay, err = f.service.CloseCancellation(ctx, f.principal, cancellation.ID, CloseInput{IdempotencyKey: "closure-cancellation-close"})
	if err != nil || !replay || closedCancellation.Status != "closed" {
		t.Fatalf("cancellation replay=%#v replay=%v err=%v", closedCancellation, replay, err)
	}
	notes := "changed"
	if _, _, err = f.service.CloseCancellation(ctx, f.principal, cancellation.ID, CloseInput{Notes: &notes, IdempotencyKey: "closure-cancellation-close"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("cancellation changed replay=%v", err)
	}
	if _, _, err = f.service.CloseCancellation(ctx, f.principal, cancellation.ID, CloseInput{IdempotencyKey: "closure-cancellation-second"}); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("second cancellation close=%v", err)
	}
	if _, _, err = f.service.CloseCancellation(ctx, f.principal, f.otherOrder, CloseInput{IdempotencyKey: "closure-cancellation-cross"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant cancellation close=%v", err)
	}
	if _, err = f.db.Exec(ctx, `UPDATE cancellation_events SET notes='rewrite' WHERE company_id=$1 AND cancellation_id=$2`, f.company, cancellation.ID); err == nil {
		t.Fatal("cancellation event history update was allowed")
	}

	item, _, err := f.service.CreateReturn(ctx, f.principal, CreateReturnInput{MarketplaceOrderID: f.flipOrder, Reason: "Damaged physical return", Items: []ExpectedItemInput{{MarketplaceOrderItemID: f.flipItem, ExpectedQuantity: 1}}, IdempotencyKey: "closure-return-create"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.service.CloseReturn(ctx, f.principal, item.ID, CloseInput{IdempotencyKey: "closure-return-too-early"}); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("early return close=%v", err)
	}
	item, _, err = f.service.ReceiveReturn(ctx, f.principal, item.ID, ReceiveReturnInput{Items: []ReceivedItemInput{{ReturnItemID: item.Items[0].ID, ReceivedQuantity: 1}}, IdempotencyKey: "closure-return-receive"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.InspectReturn(ctx, f.principal, item.ID, InspectReturnInput{Items: []DispositionItemInput{{ReturnItemID: item.Items[0].ID, Disposition: "damaged"}}, IdempotencyKey: "closure-return-inspect"})
	if err != nil || item.Status != "damaged" {
		t.Fatalf("inspection=%#v err=%v", item, err)
	}
	item, replay, err = f.service.CloseReturn(ctx, f.principal, item.ID, CloseInput{IdempotencyKey: "closure-return-close"})
	if err != nil || replay || item.Status != "closed" || item.ClosedBy == nil || item.ClosedAt == nil || item.Events[len(item.Events)-1].EventType != "closed" {
		t.Fatalf("closed return=%#v replay=%v err=%v", item, replay, err)
	}
	item, replay, err = f.service.CloseReturn(ctx, f.principal, item.ID, CloseInput{IdempotencyKey: "closure-return-close"})
	if err != nil || !replay || item.Status != "closed" {
		t.Fatalf("return replay=%#v replay=%v err=%v", item, replay, err)
	}
	var inventoryTransactions int
	mustScan(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, []any{f.company}, &inventoryTransactions)
	if inventoryTransactions != 0 {
		t.Fatalf("closure changed inventory transactions=%d", inventoryTransactions)
	}
}

func TestConcurrentReturnClosureIsExactReplay(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	item, _, err := f.service.CreateReturn(ctx, f.principal, CreateReturnInput{MarketplaceOrderID: f.concurrentOrder, Reason: "Concurrent closure", Items: []ExpectedItemInput{{MarketplaceOrderItemID: f.concurrentItem, ExpectedQuantity: 1}}, IdempotencyKey: "concurrent-close-create"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.ReceiveReturn(ctx, f.principal, item.ID, ReceiveReturnInput{Items: []ReceivedItemInput{{ReturnItemID: item.Items[0].ID, ReceivedQuantity: 1}}, IdempotencyKey: "concurrent-close-receive"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.InspectReturn(ctx, f.principal, item.ID, InspectReturnInput{Items: []DispositionItemInput{{ReturnItemID: item.Items[0].ID, Disposition: "damaged"}}, IdempotencyKey: "concurrent-close-inspect"})
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		replay bool
		err    error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, replay, runErr := f.service.CloseReturn(ctx, f.principal, item.ID, CloseInput{IdempotencyKey: "concurrent-close"})
			results <- result{replay: replay, err: runErr}
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	created, replayed := 0, 0
	for outcome := range results {
		if outcome.err != nil {
			t.Fatal(outcome.err)
		}
		if outcome.replay {
			replayed++
		} else {
			created++
		}
	}
	var events int
	mustScan(t, f.db, `SELECT count(*) FROM return_events WHERE company_id=$1 AND return_case_id=$2 AND event_type='closed'`, []any{f.company, item.ID}, &events)
	if created != 1 || replayed != 1 || events != 1 {
		t.Fatalf("created=%d replayed=%d events=%d", created, replayed, events)
	}
}

func TestPhaseEightReportingMetricsAndAccess(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	for _, input := range []CreateCancellationInput{
		{MarketplaceOrderID: f.flipOrder, Reason: "Flipkart cancellation", CancelledAt: time.Now(), IdempotencyKey: "metrics-cancel-flipkart"},
		{MarketplaceOrderID: f.amazonOrder, Reason: "Amazon cancellation", CancelledAt: time.Now(), IdempotencyKey: "metrics-cancel-amazon"},
	} {
		item, _, err := f.service.CreateCancellation(ctx, f.principal, input)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err = f.service.CloseCancellation(ctx, f.principal, item.ID, CloseInput{IdempotencyKey: input.IdempotencyKey + "-close"}); err != nil {
			t.Fatal(err)
		}
	}

	restocked := completeInspectedReturn(t, f, f.amazonItem, 2, "restockable", "metrics-restock")
	restocked, _, err := f.service.RestockReturn(ctx, f.principal, restocked.ID, RestockReturnInput{IdempotencyKey: "metrics-restock-stock"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.service.CorrectRestock(ctx, f.principal, restocked.ID, CorrectRestockInput{Items: []CorrectionItemInput{{ReturnItemID: restocked.Items[0].ID, Quantity: 1}}, Reason: "One unit failed final check", IdempotencyKey: "metrics-restock-correct"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.service.CloseReturn(ctx, f.principal, restocked.ID, CloseInput{IdempotencyKey: "metrics-restock-close"}); err != nil {
		t.Fatal(err)
	}
	damaged := completeInspectedReturn(t, f, f.amazonSecondItem, 2, "damaged", "metrics-damaged")
	if _, _, err = f.service.CloseReturn(ctx, f.principal, damaged.ID, CloseInput{IdempotencyKey: "metrics-damaged-close"}); err != nil {
		t.Fatal(err)
	}

	reportingService := reporting.NewService(f.db, authorization.NewService(f.db))
	filter := reporting.Filter{From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), Marketplace: "amazon", Limit: 50}
	report, err := reportingService.Dashboard(ctx, f.principal, filter)
	if err != nil {
		t.Fatal(err)
	}
	if !report.ReturnsAccess || report.Returns == nil {
		t.Fatalf("returns access=%v summary=%#v", report.ReturnsAccess, report.Returns)
	}
	metrics := report.Returns
	if metrics.Cancellations != 1 || metrics.ClosedCancellations != 1 || metrics.ReturnsReceived != 2 || metrics.ReceivedQuantity != 4 || metrics.RestockedQuantity != 2 || metrics.DamagedQuantity != 2 || metrics.ClosedReturns != 2 || metrics.CohortReturnedOrders != 1 || metrics.CohortResolvedOrders != 2 || metrics.CohortReturnRatePercent != 50 {
		t.Fatalf("Amazon return metrics=%#v", metrics)
	}
	if report.Inventory == nil || report.Inventory.ReturnRestock != 1 {
		t.Fatalf("net inventory restock=%#v", report.Inventory)
	}
	filter.Marketplace = "flipkart"
	report, err = reportingService.Dashboard(ctx, f.principal, filter)
	if err != nil || report.Returns == nil || report.Returns.Cancellations != 1 || report.Returns.ReturnsReceived != 0 || report.Returns.RestockedQuantity != 0 || report.Returns.CohortResolvedOrders != 1 || report.Returns.CohortReturnRatePercent != 0 {
		t.Fatalf("Flipkart return metrics=%#v err=%v", report.Returns, err)
	}
	mustExec(t, f.db, `DELETE FROM role_permissions WHERE company_id=$1 AND role_id=$2 AND permission_key='returns.view'`, f.company, f.role)
	report, err = reportingService.Dashboard(ctx, f.principal, filter)
	if err != nil || report.ReturnsAccess || report.Returns != nil {
		t.Fatalf("restricted return metrics access=%v summary=%#v err=%v", report.ReturnsAccess, report.Returns, err)
	}
}

func completeInspectedReturn(t *testing.T, f *fixture, orderItemID string, quantity int64, disposition, key string) ReturnCase {
	t.Helper()
	item, _, err := f.service.CreateReturn(context.Background(), f.principal, CreateReturnInput{MarketplaceOrderID: f.amazonOrder, Reason: key, Items: []ExpectedItemInput{{MarketplaceOrderItemID: orderItemID, ExpectedQuantity: quantity}}, IdempotencyKey: key + "-create"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.ReceiveReturn(context.Background(), f.principal, item.ID, ReceiveReturnInput{Items: []ReceivedItemInput{{ReturnItemID: item.Items[0].ID, ReceivedQuantity: quantity}}, IdempotencyKey: key + "-receive"})
	if err != nil {
		t.Fatal(err)
	}
	item, _, err = f.service.InspectReturn(context.Background(), f.principal, item.ID, InspectReturnInput{Items: []DispositionItemInput{{ReturnItemID: item.Items[0].ID, Disposition: disposition}}, IdempotencyKey: key + "-inspect"})
	if err != nil {
		t.Fatal(err)
	}
	return item
}
