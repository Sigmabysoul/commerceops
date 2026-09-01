package marketplace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/marketplace/meesho"
	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
)

func TestMeeshoBatchAPostgreSQLIntegration(t *testing.T) {
	f := setupPhaseThree(t)
	ctx := context.Background()
	mustExecP3(t, f.db, `INSERT INTO module_entitlements(company_id,module_key,enabled) VALUES($1,'meesho',true),($2,'meesho',true)`, f.companyA, f.companyB)
	mustExecP3(t, f.db, `INSERT INTO sku_mappings(company_id,marketplace_key,product_id,sku) VALUES($1,'meesho',$2,'MEESHO-KNOWN')`, f.companyA, f.productID)
	service, err := newServiceForProcessor(f.db, authorization.NewService(f.db), f.service.storage, f.extractor, meeshoProcessor())
	if err != nil {
		t.Fatal(err)
	}

	t.Run("entitlement is required before persistence", func(t *testing.T) {
		pdf := f.register("meesho-denied", pdfextractor.Page{Number: 1, Text: meeshoText("100000000001_1", "MEESHOAWBDENIED", "MEESHO-KNOWN", "1")})
		mustExecP3(t, f.db, `UPDATE module_entitlements SET enabled=false WHERE company_id=$1 AND module_key='meesho'`, f.companyA)
		if _, uploadErr := service.Upload(ctx, f.principalA, "denied.pdf", pdf); !errors.Is(uploadErr, authorization.ErrModuleUnavailable) {
			t.Fatalf("upload error=%v", uploadErr)
		}
		mustExecP3(t, f.db, `UPDATE module_entitlements SET enabled=true WHERE company_id=$1 AND module_key='meesho'`, f.companyA)
		var count int
		mustScanP3(t, f.db, `SELECT count(*) FROM source_files WHERE company_id=$1 AND marketplace_key='meesho'`, []any{f.companyA}, &count)
		if count != 0 {
			t.Fatalf("denied Meesho sources=%d", count)
		}
	})
	t.Run("processing permissions are required before persistence", func(t *testing.T) {
		var roleID string
		mustScanP3(t, f.db, `SELECT id FROM roles WHERE company_id=$1 AND name='Flipkart Operator'`, []any{f.companyA}, &roleID)
		mustExecP3(t, f.db, `DELETE FROM role_permissions WHERE company_id=$1 AND role_id=$2 AND permission_key='labels.process'`, f.companyA, roleID)
		pdf := f.register("meesho-permission-denied", pdfextractor.Page{Number: 1, Text: meeshoText("100000000001_2", "MEESHOAWBDENIED2", "MEESHO-KNOWN", "1")})
		if _, uploadErr := service.Upload(ctx, f.principalA, "permission-denied.pdf", pdf); !errors.Is(uploadErr, authorization.ErrPermissionDenied) {
			t.Fatalf("upload error=%v", uploadErr)
		}
		mustExecP3(t, f.db, `INSERT INTO role_permissions(company_id,role_id,permission_key) VALUES($1,$2,'labels.process')`, f.companyA, roleID)
		var count int
		mustScanP3(t, f.db, `SELECT count(*) FROM source_files WHERE company_id=$1 AND marketplace_key='meesho'`, []any{f.companyA}, &count)
		if count != 0 {
			t.Fatalf("permission-denied Meesho sources=%d", count)
		}
	})

	known := f.register("meesho-known", pdfextractor.Page{Number: 6, ExtractionMethod: "text", Text: meeshoText("100000000002_1", "MEESHOAWBKNOWN", "MEESHO-KNOWN", "2")})
	var inventoryBefore int
	mustScanP3(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, []any{f.companyA}, &inventoryBefore)
	uploaded, err := service.Upload(ctx, f.principalA, "meesho-known.pdf", known)
	if err != nil || uploaded.Job.ParserVersion != meesho.ParserVersion {
		t.Fatalf("upload=%#v err=%v", uploaded, err)
	}
	if _, claimed, claimErr := f.service.claim(ctx); claimErr != nil || claimed {
		t.Fatalf("Flipkart worker claimed Meesho job: claimed=%v err=%v", claimed, claimErr)
	}
	if processed, processErr := service.processNext(); processErr != nil || !processed {
		t.Fatalf("processed=%v err=%v", processed, processErr)
	}
	details, err := service.Get(ctx, f.principalA, uploaded.Job.ID)
	if err != nil || details.Job.Status != "processed" || details.Job.ParserVersion != meesho.ParserVersion || len(details.Orders) != 1 || details.Orders[0].SourcePage != 6 || len(details.Orders[0].Documents) != 1 || details.Orders[0].Documents[0].SourcePage != 6 || details.Orders[0].Documents[0].Role != "shipping_label" || details.Orders[0].Documents[0].ExtractionMethod != "text" || details.Orders[0].Items[0].ProductID == nil || *details.Orders[0].Items[0].ProductID != f.productID || details.Orders[0].Items[0].Quantity == nil || *details.Orders[0].Items[0].Quantity != 2 {
		t.Fatalf("details=%#v err=%v", details, err)
	}
	var traceable bool
	mustScanP3(t, f.db, `SELECT o.source_file_id=j.source_file_id AND o.parser_version=j.parser_version AND f.marketplace_key='meesho' AND f.storage_key LIKE $3 AND o.extraction_metadata->>'association_method'='single_document' FROM marketplace_orders o JOIN processing_jobs j ON j.company_id=o.company_id AND j.id=o.processing_job_id JOIN source_files f ON f.company_id=o.company_id AND f.id=o.source_file_id WHERE o.company_id=$1 AND o.processing_job_id=$2`, []any{f.companyA, uploaded.Job.ID, f.companyA + "/%"}, &traceable)
	if !traceable {
		t.Fatal("Meesho source/job/order/parser traceability was not preserved")
	}
	var inventoryAfter int
	mustScanP3(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, []any{f.companyA}, &inventoryAfter)
	if inventoryAfter != inventoryBefore {
		t.Fatalf("processing changed inventory transactions from %d to %d", inventoryBefore, inventoryAfter)
	}

	if _, getErr := service.Get(ctx, f.principalB, uploaded.Job.ID); !errors.Is(getErr, ErrNotFound) {
		t.Fatalf("cross-tenant get=%v", getErr)
	}
	if _, retryErr := service.Retry(ctx, f.principalB, uploaded.Job.ID); !errors.Is(retryErr, ErrNotFound) {
		t.Fatalf("cross-tenant retry=%v", retryErr)
	}
	duplicate, err := service.Upload(ctx, f.principalA, "same-source.pdf", known)
	if err != nil || !duplicate.DuplicateSource || duplicate.Job.ID != uploaded.Job.ID {
		t.Fatalf("duplicate=%#v err=%v", duplicate, err)
	}

	t.Run("unknown SKU and missing quantity remain review values", func(t *testing.T) {
		pdf := f.register("meesho-review", pdfextractor.Page{Number: 9, Text: meeshoText("100000000003_1", "MEESHOAWBREVIEW", "MEESHO-UNKNOWN", "")})
		result, uploadErr := service.Upload(ctx, f.principalA, "review.pdf", pdf)
		if uploadErr != nil {
			t.Fatal(uploadErr)
		}
		if ok, processErr := service.processNext(); processErr != nil || !ok {
			t.Fatalf("process=%v err=%v", ok, processErr)
		}
		review, getErr := service.Get(ctx, f.principalA, result.Job.ID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		item := review.Orders[0].Items[0]
		if review.Job.Status != "needs_review" || item.ProductID != nil || item.Quantity != nil || item.QuantitySource != "missing" || item.ResolutionStatus != "unresolved" {
			t.Fatalf("review=%#v", review)
		}
	})

	t.Run("retry uses newly trained exact Meesho SKU mapping", func(t *testing.T) {
		pdf := f.register("meesho-retry", pdfextractor.Page{Number: 3, Text: meeshoText("100000000004_1", "MEESHOAWBRETRY", "MEESHO-RETRY", "4")})
		result, uploadErr := service.Upload(ctx, f.principalA, "retry.pdf", pdf)
		if uploadErr != nil {
			t.Fatal(uploadErr)
		}
		if _, processErr := service.processNext(); processErr != nil {
			t.Fatal(processErr)
		}
		mustExecP3(t, f.db, `INSERT INTO sku_mappings(company_id,marketplace_key,product_id,sku) VALUES($1,'meesho',$2,'MEESHO-RETRY')`, f.companyA, f.productID)
		job, retryErr := service.Retry(ctx, f.principalA, result.Job.ID)
		if retryErr != nil || job.Status != "queued" || job.ParserVersion != meesho.ParserVersion {
			t.Fatalf("job=%#v err=%v", job, retryErr)
		}
		if _, processErr := service.processNext(); processErr != nil {
			t.Fatal(processErr)
		}
		after, getErr := service.Get(ctx, f.principalA, result.Job.ID)
		if getErr != nil || after.Job.Status != "processed" || after.Orders[0].Items[0].ProductID == nil {
			t.Fatalf("after=%#v err=%v", after, getErr)
		}
	})

	t.Run("duplicate Meesho business identifier is visible", func(t *testing.T) {
		pdf := f.register("meesho-business-duplicate", pdfextractor.Page{Number: 2, Text: meeshoText("100000000002_1", "MEESHOAWBOTHER", "MEESHO-KNOWN", "1")})
		result, uploadErr := service.Upload(ctx, f.principalA, "business-duplicate.pdf", pdf)
		if uploadErr != nil {
			t.Fatal(uploadErr)
		}
		if _, processErr := service.processNext(); processErr != nil {
			t.Fatal(processErr)
		}
		after, getErr := service.Get(ctx, f.principalA, result.Job.ID)
		if getErr != nil || after.Job.Status != "needs_review" || after.Orders[0].Status != "duplicate" {
			t.Fatalf("duplicate=%#v err=%v", after, getErr)
		}
	})
}

func TestMeeshoProcessingMigrationUpDown(t *testing.T) {
	f := setupPhaseThree(t)
	ctx := context.Background()
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	root := filepath.Join("..", "..", "migrations")
	down, err := os.ReadFile(filepath.Join(root, "000018_meesho_processing.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	var exists *string
	if err = tx.QueryRow(ctx, `SELECT to_regclass('processing_jobs_meesho_claim_idx')`).Scan(&exists); err != nil || exists != nil {
		t.Fatalf("down=%v err=%v", exists, err)
	}
	up, err := os.ReadFile(filepath.Join(root, "000018_meesho_processing.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(up)); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `SELECT to_regclass('processing_jobs_meesho_claim_idx')`).Scan(&exists); err != nil || exists == nil {
		t.Fatalf("up=%v err=%v", exists, err)
	}
}

func meeshoText(orderID, awb, sku, quantity string) string {
	text := "Meesho\nShipping Label\nSub Order No: " + orderID + "\nAWB Number: " + awb + "\nSupplier SKU: " + sku
	if quantity != "" {
		text += "\nQuantity: " + quantity
	}
	return text
}
