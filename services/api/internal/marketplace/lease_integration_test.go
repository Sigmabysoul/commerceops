// This file contains PostgreSQL-backed tests for cross-layer behavior, tenant isolation, and domain invariants in the marketplace orchestration package.
package marketplace

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
)

func TestFlipkartQueuedJobHasSingleLeaseOwner(t *testing.T) {
	f := setupPhaseThree(t)
	ctx := context.Background()
	pdf := f.register("lease-single-owner", pdfextractor.Page{Number: 1, Text: "Flipkart AWB: AWBLEASE1 Order ID: ODLEASE1 SKU: KNOWN-SKU Qty: 1"})
	uploaded, err := f.service.Upload(ctx, f.principalA, "lease.pdf", pdf)
	if err != nil {
		t.Fatal(err)
	}

	workers := []*Service{newLeaseTestWorker(f, "worker-a"), newLeaseTestWorker(f, "worker-b")}
	type result struct {
		item work
		ok   bool
		err  error
	}
	results := make(chan result, len(workers))
	var group sync.WaitGroup
	for _, worker := range workers {
		group.Add(1)
		go func(service *Service) {
			defer group.Done()
			item, ok, claimErr := service.claim(ctx)
			results <- result{item: item, ok: ok, err: claimErr}
		}(worker)
	}
	group.Wait()
	close(results)

	claimed := 0
	var claimedWorker string
	for result := range results {
		if result.err != nil {
			t.Fatalf("claim error = %v", result.err)
		}
		if result.ok {
			claimed++
			claimedWorker = result.item.WorkerID
			if result.item.JobID != uploaded.Job.ID {
				t.Fatalf("claimed job = %s, want %s", result.item.JobID, uploaded.Job.ID)
			}
		}
	}
	if claimed != 1 {
		t.Fatalf("successful claims = %d, want 1", claimed)
	}

	var status string
	var owner *string
	var leaseValid bool
	mustScanP3(t, f.db, `SELECT status,worker_id,lease_expires_at>now() FROM processing_jobs WHERE id=$1`, []any{uploaded.Job.ID}, &status, &owner, &leaseValid)
	if status != "processing" || owner == nil || *owner != claimedWorker || !leaseValid {
		t.Fatalf("status=%s owner=%v claimed=%s lease_valid=%v", status, owner, claimedWorker, leaseValid)
	}
	if _, err = workers[0].Retry(ctx, f.principalA, uploaded.Job.ID); !errors.Is(err, ErrJobActive) {
		t.Fatalf("Retry() error = %v, want ErrJobActive", err)
	}
}

func TestFlipkartLeaseExpiryAndRenewal(t *testing.T) {
	f := setupPhaseThree(t)
	ctx := context.Background()
	pdf := f.register("lease-expiry", pdfextractor.Page{Number: 1, Text: "Flipkart AWB: AWBLEASE2 Order ID: ODLEASE2 SKU: KNOWN-SKU Qty: 1"})
	uploaded, err := f.service.Upload(ctx, f.principalA, "lease-expiry.pdf", pdf)
	if err != nil {
		t.Fatal(err)
	}
	workerA := newLeaseTestWorker(f, "worker-a")
	workerB := newLeaseTestWorker(f, "worker-b")
	itemA, ok, err := workerA.claim(ctx)
	if err != nil || !ok {
		t.Fatalf("worker A claim = %#v, ok=%v, err=%v", itemA, ok, err)
	}
	if _, ok, err = workerB.claim(ctx); err != nil || ok {
		t.Fatalf("worker B stole valid lease: ok=%v err=%v", ok, err)
	}

	mustExecP3(t, f.db, `UPDATE processing_jobs SET lease_expires_at=now()-interval '1 second' WHERE id=$1`, uploaded.Job.ID)
	itemB, ok, err := workerB.claim(ctx)
	if err != nil || !ok || itemB.JobID != uploaded.Job.ID || itemB.WorkerID != "worker-b" {
		t.Fatalf("expired reclaim = %#v, ok=%v, err=%v", itemB, ok, err)
	}
	if err = workerA.renewLease(ctx, itemA); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale renewal error = %v, want ErrLeaseLost", err)
	}

	mustExecP3(t, f.db, `UPDATE processing_jobs SET lease_expires_at=now()+interval '1 second' WHERE id=$1`, uploaded.Job.ID)
	if err = workerB.renewLease(ctx, itemB); err != nil {
		t.Fatalf("renewLease() error = %v", err)
	}
	var renewed bool
	mustScanP3(t, f.db, `SELECT worker_id=$2 AND lease_expires_at>now()+interval '90 seconds' FROM processing_jobs WHERE id=$1`, []any{uploaded.Job.ID, "worker-b"}, &renewed)
	if !renewed {
		t.Fatal("lease was not renewed to the configured duration")
	}
	if _, ok, err = workerA.claim(ctx); err != nil || ok {
		t.Fatalf("renewed lease was reclaimable: ok=%v err=%v", ok, err)
	}
	if err = workerA.execute(ctx, itemA); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale execute error = %v, want ErrLeaseLost", err)
	}
	var orderCount int
	mustScanP3(t, f.db, `SELECT count(*) FROM marketplace_orders WHERE company_id=$1 AND processing_job_id=$2`, []any{f.companyA, uploaded.Job.ID}, &orderCount)
	if orderCount != 0 {
		t.Fatalf("stale worker created %d orders", orderCount)
	}
	if err = workerB.execute(ctx, itemB); err != nil {
		t.Fatalf("current owner execute error = %v", err)
	}
	mustScanP3(t, f.db, `SELECT count(*) FROM marketplace_orders WHERE company_id=$1 AND processing_job_id=$2`, []any{f.companyA, uploaded.Job.ID}, &orderCount)
	if orderCount != 1 {
		t.Fatalf("authoritative order count = %d, want 1", orderCount)
	}
	assertLeaseCleared(t, f.db, uploaded.Job.ID)
}

func TestFlipkartRecoveryIsLeaseAwareAndMarketplaceScoped(t *testing.T) {
	f := setupPhaseThree(t)
	ctx := context.Background()
	var sourceFlipkart, sourceAmazon, jobFlipkart, jobAmazon string
	insert := func(marketplace, owner string, sourceID, jobID *string) {
		t.Helper()
		mustScanP3(t, f.db, `INSERT INTO source_files(company_id,marketplace_key,storage_key,original_filename,content_type,size_bytes,sha256,uploaded_by) VALUES($1,$2,$3,'x.pdf','application/pdf',1,$4,$5) RETURNING id`, []any{f.companyA, marketplace, "x/" + marketplace, fmt.Sprintf("%064x", marketplace), f.userID}, sourceID)
		mustScanP3(t, f.db, `INSERT INTO processing_jobs(company_id,source_file_id,marketplace_key,status,parser_version,started_at,worker_id,lease_expires_at) VALUES($1,$2,$3,'processing','test',now(),$4,now()+interval '10 minutes') RETURNING id`, []any{f.companyA, *sourceID, marketplace, owner}, jobID)
	}
	insert("flipkart", "healthy-flipkart-worker", &sourceFlipkart, &jobFlipkart)
	insert("amazon", "healthy-amazon-worker", &sourceAmazon, &jobAmazon)

	worker := newLeaseTestWorker(f, "replacement-worker")
	worker.recoverJobs()
	assertJobLease(t, f, jobFlipkart, "processing", "healthy-flipkart-worker")
	assertJobLease(t, f, jobAmazon, "processing", "healthy-amazon-worker")
	if _, ok, err := worker.claim(ctx); err != nil || ok {
		t.Fatalf("claim with healthy leases: ok=%v err=%v", ok, err)
	}

	mustExecP3(t, f.db, `UPDATE processing_jobs SET lease_expires_at=now()-interval '1 second' WHERE id=$1`, jobFlipkart)
	worker.recoverJobs()
	claimed, ok, err := worker.claim(ctx)
	if err != nil || !ok || claimed.JobID != jobFlipkart {
		t.Fatalf("expired Flipkart claim = %#v, ok=%v, err=%v", claimed, ok, err)
	}
	assertJobLease(t, f, jobAmazon, "processing", "healthy-amazon-worker")
}

func TestConcurrentFlipkartWorkersCreateOneAuthoritativeOrder(t *testing.T) {
	f := setupPhaseThree(t)
	ctx := context.Background()
	pdf := f.register("lease-concurrent-process", pdfextractor.Page{Number: 1, Text: "Flipkart AWB: AWBLEASE3 Order ID: ODLEASE3 SKU: KNOWN-SKU Qty: 1"})
	uploaded, err := f.service.Upload(ctx, f.principalA, "concurrent.pdf", pdf)
	if err != nil {
		t.Fatal(err)
	}
	workers := []*Service{newLeaseTestWorker(f, "worker-a"), newLeaseTestWorker(f, "worker-b")}
	type result struct {
		processed bool
		err       error
	}
	results := make(chan result, len(workers))
	var group sync.WaitGroup
	for _, worker := range workers {
		group.Add(1)
		go func(service *Service) {
			defer group.Done()
			processed, processErr := service.processNext()
			results <- result{processed: processed, err: processErr}
		}(worker)
	}
	group.Wait()
	close(results)

	processedCount := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("processNext() error = %v", result.err)
		}
		if result.processed {
			processedCount++
		}
	}
	if processedCount != 1 {
		t.Fatalf("workers reporting processed work = %d, want 1", processedCount)
	}
	var orderCount int
	var status string
	mustScanP3(t, f.db, `SELECT count(*) FROM marketplace_orders WHERE company_id=$1 AND processing_job_id=$2 AND marketplace_key='flipkart'`, []any{f.companyA, uploaded.Job.ID}, &orderCount)
	mustScanP3(t, f.db, `SELECT status FROM processing_jobs WHERE id=$1`, []any{uploaded.Job.ID}, &status)
	if orderCount != 1 || status != "processed" {
		t.Fatalf("order_count=%d status=%s", orderCount, status)
	}
	assertLeaseCleared(t, f.db, uploaded.Job.ID)
}

func newLeaseTestWorker(f *phaseThreeFixture, workerID string) *Service {
	return newServiceWithWorkerID(f.db, f.service.authorizer, f.service.storage, f.extractor, workerID)
}

func assertJobLease(t *testing.T, f *phaseThreeFixture, jobID, wantStatus, wantWorker string) {
	t.Helper()
	var status string
	var workerID *string
	var leaseValid bool
	mustScanP3(t, f.db, `SELECT status,worker_id,lease_expires_at>now() FROM processing_jobs WHERE id=$1`, []any{jobID}, &status, &workerID, &leaseValid)
	if status != wantStatus || workerID == nil || *workerID != wantWorker || !leaseValid {
		t.Fatalf("job=%s status=%s worker=%v lease_valid=%v", jobID, status, workerID, leaseValid)
	}
}
