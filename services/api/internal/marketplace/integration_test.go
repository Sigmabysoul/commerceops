package marketplace

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
	"github.com/commerceops/commerceops/services/api/internal/platform/objectstorage"
	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
	"github.com/jackc/pgx/v5/pgxpool"
)

type mappedExtractor struct {
	mu    sync.RWMutex
	pages map[string][]pdfextractor.Page
	fail  map[string]error
}

func (e *mappedExtractor) Extract(_ context.Context, pdf []byte) ([]pdfextractor.Page, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if err := e.fail[string(pdf)]; err != nil {
		return nil, err
	}
	pages, ok := e.pages[string(pdf)]
	if !ok {
		return nil, errors.New("fixture missing")
	}
	return pages, nil
}

type phaseThreeFixture struct {
	db                                    *pgxpool.Pool
	service                               *Service
	extractor                             *mappedExtractor
	companyA, companyB, userID, productID string
	principalA, principalB                auth.Principal
}

func setupPhaseThree(t *testing.T) *phaseThreeFixture {
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
	suffix := fmt.Sprint(time.Now().UnixNano())
	f := &phaseThreeFixture{db: db, extractor: &mappedExtractor{pages: map[string][]pdfextractor.Page{}, fail: map[string]error{}}}
	scan := func(query string, args []any, dest ...any) {
		t.Helper()
		if err := db.QueryRow(ctx, query, args...).Scan(dest...); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}
	scan(`INSERT INTO companies(name) VALUES($1) RETURNING id`, []any{"P3 A " + suffix}, &f.companyA)
	scan(`INSERT INTO companies(name) VALUES($1) RETURNING id`, []any{"P3 B " + suffix}, &f.companyB)
	scan(`INSERT INTO users(email,password_hash) VALUES($1,'test') RETURNING id`, []any{"p3-" + suffix + "@example.test"}, &f.userID)
	mustExecP3(t, db, `INSERT INTO company_users(company_id,user_id) VALUES($1,$3),($2,$3)`, f.companyA, f.companyB, f.userID)
	for _, company := range []string{f.companyA, f.companyB} {
		var role string
		scan(`INSERT INTO roles(company_id,name) VALUES($1,'Flipkart Operator') RETURNING id`, []any{company}, &role)
		mustExecP3(t, db, `INSERT INTO role_permissions(company_id,role_id,permission_key) VALUES($1,$2,'labels.upload'),($1,$2,'labels.process')`, company, role)
		mustExecP3(t, db, `INSERT INTO company_user_roles(company_id,user_id,role_id) VALUES($1,$2,$3)`, company, f.userID, role)
		mustExecP3(t, db, `INSERT INTO module_entitlements(company_id,module_key,enabled) VALUES($1,'flipkart',true)`, company)
	}
	scan(`INSERT INTO products(company_id,internal_code,name) VALUES($1,'KNOWN','Known Product') RETURNING id`, []any{f.companyA}, &f.productID)
	mustExecP3(t, db, `INSERT INTO sku_mappings(company_id,marketplace_key,product_id,sku) VALUES($1,'flipkart',$2,'KNOWN-SKU')`, f.companyA, f.productID)
	store, err := objectstorage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f.service = newService(db, authorization.NewService(db), store, f.extractor)
	f.principalA = auth.Principal{CompanyID: f.companyA, UserID: f.userID}
	f.principalB = auth.Principal{CompanyID: f.companyB, UserID: f.userID}
	t.Cleanup(func() { cleanupPhaseThree(t, f); db.Close() })
	return f
}
func (f *phaseThreeFixture) register(data string, pages ...pdfextractor.Page) []byte {
	pdf := []byte("%PDF-" + data)
	f.extractor.mu.Lock()
	f.extractor.pages[string(pdf)] = pages
	f.extractor.mu.Unlock()
	return pdf
}
func (f *phaseThreeFixture) process(t *testing.T) {
	t.Helper()
	processed, err := f.service.processNext()
	if err != nil {
		t.Fatal(err)
	}
	if !processed {
		t.Fatal("expected queued job")
	}
}

func TestPhaseThreePostgreSQLBehavior(t *testing.T) {
	f := setupPhaseThree(t)
	ctx := context.Background()
	known := f.register("known", pdfextractor.Page{Number: 1, Text: "Flipkart AWB: AWBKNOWN1 Order ID: ODKNOWN1 SKU: KNOWN-SKU Qty: 2"})
	uploaded, err := f.service.Upload(ctx, f.principalA, "known.pdf", known)
	if err != nil {
		t.Fatal(err)
	}
	f.process(t)
	details, err := f.service.Get(ctx, f.principalA, uploaded.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if details.Job.Status != "processed" || len(details.Orders) != 1 || details.Orders[0].SourcePage != 1 || details.Orders[0].Items[0].ProductID == nil || *details.Orders[0].Items[0].ProductID != f.productID {
		t.Fatalf("resolved result=%#v", details)
	}
	t.Run("tenant get and retry isolation", func(t *testing.T) {
		if _, err := f.service.Get(ctx, f.principalB, uploaded.Job.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("get err=%v", err)
		}
		if _, err := f.service.Retry(ctx, f.principalB, uploaded.Job.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("retry err=%v", err)
		}
	})
	t.Run("source duplicate is per tenant", func(t *testing.T) {
		duplicate, err := f.service.Upload(ctx, f.principalA, "again.pdf", known)
		if err != nil || !duplicate.DuplicateSource || duplicate.Job.ID != uploaded.Job.ID {
			t.Fatalf("same tenant duplicate=%#v err=%v", duplicate, err)
		}
		other, err := f.service.Upload(ctx, f.principalB, "same.pdf", known)
		if err != nil || other.DuplicateSource || other.Job.ID == uploaded.Job.ID {
			t.Fatalf("other tenant=%#v err=%v", other, err)
		}
		f.process(t)
	})
	t.Run("concurrent source duplicate race", func(t *testing.T) {
		pdf := f.register("race", pdfextractor.Page{Number: 1, Text: "Flipkart AWB: AWBRACE01 Order ID: ODRACE01 SKU: KNOWN-SKU Qty: 1"})
		const count = 8
		results := make(chan UploadResult, count)
		errs := make(chan error, count)
		var wg sync.WaitGroup
		for range count {
			wg.Add(1)
			go func() {
				defer wg.Done()
				result, err := f.service.Upload(ctx, f.principalA, "race.pdf", pdf)
				if err != nil {
					errs <- err
					return
				}
				results <- result
			}()
		}
		wg.Wait()
		close(errs)
		close(results)
		for err := range errs {
			t.Errorf("upload: %v", err)
		}
		ids := map[string]bool{}
		duplicates := 0
		for result := range results {
			ids[result.Job.ID] = true
			if result.DuplicateSource {
				duplicates++
			}
		}
		if len(ids) != 1 || duplicates != count-1 {
			t.Fatalf("ids=%v duplicates=%d", ids, duplicates)
		}
		f.process(t)
	})
	t.Run("unresolved and missing quantity remain review null", func(t *testing.T) {
		pdf := f.register("unresolved", pdfextractor.Page{Number: 3, Text: "Flipkart AWB: AWBUNKNOWN Order ID: ODUNKNOWN SKU: UNKNOWN"})
		result, err := f.service.Upload(ctx, f.principalA, "unknown.pdf", pdf)
		if err != nil {
			t.Fatal(err)
		}
		f.process(t)
		details, err := f.service.Get(ctx, f.principalA, result.Job.ID)
		if err != nil {
			t.Fatal(err)
		}
		item := details.Orders[0].Items[0]
		if details.Job.Status != "needs_review" || item.Quantity != nil || item.QuantitySource != "missing" || item.ResolutionStatus != "unresolved" || details.Orders[0].SourcePage != 3 {
			t.Fatalf("details=%#v", details)
		}
	})
	t.Run("duplicate AWB and order identifiers are visible", func(t *testing.T) {
		for index, text := range []string{"Flipkart AWB: AWBKNOWN1 Order ID: ODOTHER1 SKU: KNOWN-SKU Qty: 1", "Flipkart AWB: AWBOTHER1 Order ID: ODKNOWN1 SKU: KNOWN-SKU Qty: 1"} {
			pdf := f.register(fmt.Sprintf("duplicate-%d", index), pdfextractor.Page{Number: index + 1, Text: text})
			result, err := f.service.Upload(ctx, f.principalA, "duplicate.pdf", pdf)
			if err != nil {
				t.Fatal(err)
			}
			f.process(t)
			details, err := f.service.Get(ctx, f.principalA, result.Job.ID)
			if err != nil {
				t.Fatal(err)
			}
			if details.Job.Status != "needs_review" || details.Orders[0].Status != "duplicate" {
				t.Fatalf("duplicate=%#v", details)
			}
		}
	})
	t.Run("safe retry and worker transitions", func(t *testing.T) {
		pdf := f.register("retry", pdfextractor.Page{Number: 1, Text: "Flipkart AWB: AWBRETRY1 Order ID: ODRETRY1 SKU: RETRY-SKU Qty: 1"})
		result, err := f.service.Upload(ctx, f.principalA, "retry.pdf", pdf)
		if err != nil {
			t.Fatal(err)
		}
		f.process(t)
		before, _ := f.service.Get(ctx, f.principalA, result.Job.ID)
		if before.Job.Status != "needs_review" {
			t.Fatalf("before=%s", before.Job.Status)
		}
		mustExecP3(t, f.db, `INSERT INTO sku_mappings(company_id,marketplace_key,product_id,sku) VALUES($1,'flipkart',$2,'RETRY-SKU')`, f.companyA, f.productID)
		retried, err := f.service.Retry(ctx, f.principalA, result.Job.ID)
		if err != nil || retried.Status != "queued" {
			t.Fatalf("retry=%#v err=%v", retried, err)
		}
		f.process(t)
		after, _ := f.service.Get(ctx, f.principalA, result.Job.ID)
		if after.Job.Status != "processed" {
			t.Fatalf("after=%#v", after)
		}
	})
	t.Run("extractor failure becomes failed with error", func(t *testing.T) {
		pdf := []byte("%PDF-failure")
		f.extractor.mu.Lock()
		f.extractor.fail[string(pdf)] = errors.New("broken fixture")
		f.extractor.mu.Unlock()
		result, err := f.service.Upload(ctx, f.principalA, "failure.pdf", pdf)
		if err != nil {
			t.Fatal(err)
		}
		f.process(t)
		details, _ := f.service.Get(ctx, f.principalA, result.Job.ID)
		if details.Job.Status != "failed" || len(details.Errors) == 0 || details.Errors[0].Code != "PDF_EXTRACTION_FAILED" {
			t.Fatalf("failure=%#v", details)
		}
	})
}

func TestFlipkartRecoveryAndClaimNeverTouchOtherMarketplace(t *testing.T) {
	f := setupPhaseThree(t)
	ctx := context.Background()
	var sourceFlip, sourceAmazon, jobFlip, jobAmazon string
	insert := func(market string, source, job *string) {
		mustScanP3(t, f.db, `INSERT INTO source_files(company_id,marketplace_key,storage_key,original_filename,content_type,size_bytes,sha256,uploaded_by) VALUES($1,$2,$3,'x.pdf','application/pdf',1,$4,$5) RETURNING id`, []any{f.companyA, market, "x/" + market, fmt.Sprintf("%064x", market), f.userID}, source)
		mustScanP3(t, f.db, `INSERT INTO processing_jobs(company_id,source_file_id,marketplace_key,status,parser_version,started_at) VALUES($1,$2,$3,'processing','test',now()) RETURNING id`, []any{f.companyA, *source, market}, job)
	}
	insert("flipkart", &sourceFlip, &jobFlip)
	insert("amazon", &sourceAmazon, &jobAmazon)
	f.service.recoverJobs()
	var flipStatus, amazonStatus string
	mustScanP3(t, f.db, `SELECT status FROM processing_jobs WHERE id=$1`, []any{jobFlip}, &flipStatus)
	mustScanP3(t, f.db, `SELECT status FROM processing_jobs WHERE id=$1`, []any{jobAmazon}, &amazonStatus)
	if flipStatus != "queued" || amazonStatus != "processing" {
		t.Fatalf("flipkart=%s amazon=%s", flipStatus, amazonStatus)
	}
	claimed, ok, err := f.service.claim(ctx)
	if err != nil || !ok || claimed.JobID != jobFlip {
		t.Fatalf("claim=%#v ok=%v err=%v", claimed, ok, err)
	}
	mustScanP3(t, f.db, `SELECT status FROM processing_jobs WHERE id=$1`, []any{jobAmazon}, &amazonStatus)
	if amazonStatus != "processing" {
		t.Fatalf("amazon was changed: %s", amazonStatus)
	}
}

func TestPhaseThreeMigrationUpDown(t *testing.T) {
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
	schema := "p3_migration_" + fmt.Sprint(time.Now().UnixNano())
	if _, err = tx.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SET LOCAL search_path TO `+schema+`,public`); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("..", "..", "migrations")
	for _, name := range []string{"000001_core_platform.up.sql", "000002_tenant_sessions.up.sql", "000003_product_master.up.sql", "000004_flipkart_processing.up.sql"} {
		sql, readErr := os.ReadFile(filepath.Join(root, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = tx.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	var exists *string
	if err = tx.QueryRow(ctx, `SELECT to_regclass($1)`, schema+".processing_jobs").Scan(&exists); err != nil || exists == nil {
		t.Fatalf("up verification: %v %v", exists, err)
	}
	down, err := os.ReadFile(filepath.Join(root, "000004_flipkart_processing.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(down)); err != nil {
		t.Fatalf("down: %v", err)
	}
	exists = nil
	if err = tx.QueryRow(ctx, `SELECT to_regclass($1)`, schema+".processing_jobs").Scan(&exists); err != nil || exists != nil {
		t.Fatalf("down verification: %v %v", exists, err)
	}
}

func mustExecP3(t *testing.T, db *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("fixture exec: %v", err)
	}
}
func mustScanP3(t *testing.T, db *pgxpool.Pool, query string, args []any, dest ...any) {
	t.Helper()
	if err := db.QueryRow(context.Background(), query, args...).Scan(dest...); err != nil {
		t.Fatalf("fixture scan: %v", err)
	}
}
func cleanupPhaseThree(t *testing.T, f *phaseThreeFixture) {
	t.Helper()
	ctx := context.Background()
	companies := []string{f.companyA, f.companyB}
	for _, table := range []string{"marketplace_order_items", "processing_errors", "marketplace_orders", "processing_jobs", "source_files", "sku_mappings", "products", "audit_logs", "sessions", "module_entitlements", "company_user_roles", "role_permissions", "employees", "roles", "company_users"} {
		query := "DELETE FROM " + table + " WHERE company_id=ANY($1::uuid[])"
		if table == "marketplace_order_items" {
			query = `DELETE FROM marketplace_order_items WHERE order_id IN(SELECT id FROM marketplace_orders WHERE company_id=ANY($1::uuid[]))`
		}
		if _, err := f.db.Exec(ctx, query, companies); err != nil {
			t.Errorf("cleanup %s: %v", table, err)
		}
	}
	_, _ = f.db.Exec(ctx, `DELETE FROM users WHERE id=$1`, f.userID)
	_, _ = f.db.Exec(ctx, `DELETE FROM companies WHERE id=ANY($1::uuid[])`, companies)
}
