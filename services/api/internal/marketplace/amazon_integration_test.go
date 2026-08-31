package marketplace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/marketplace/amazon"
	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
)

func TestAmazonBatchBPostgreSQLIntegration(t *testing.T) {
	f := setupPhaseThree(t)
	ctx := context.Background()
	mustExecP3(t, f.db, `INSERT INTO module_entitlements(company_id,module_key,enabled) VALUES($1,'amazon',true),($2,'amazon',true)`, f.companyA, f.companyB)
	mustExecP3(t, f.db, `INSERT INTO sku_mappings(company_id,marketplace_key,product_id,sku) VALUES($1,'amazon',$2,'AMZ-KNOWN')`, f.companyA, f.productID)
	service, err := newServiceForProcessor(f.db, authorization.NewService(f.db), f.service.storage, f.extractor, amazonProcessor())
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Amazon entitlement is required before persistence", func(t *testing.T) {
		pdf := f.register("amazon-denied", pdfextractor.Page{Number: 1, Text: amazonText("406-1000000-1000000", "TRACKDENIED1", "AMZ-KNOWN", "1")})
		mustExecP3(t, f.db, `UPDATE module_entitlements SET enabled=false WHERE company_id=$1 AND module_key='amazon'`, f.companyA)
		if _, uploadErr := service.Upload(ctx, f.principalA, "denied.pdf", pdf); !errors.Is(uploadErr, authorization.ErrModuleUnavailable) {
			t.Fatalf("upload error=%v", uploadErr)
		}
		mustExecP3(t, f.db, `UPDATE module_entitlements SET enabled=true WHERE company_id=$1 AND module_key='amazon'`, f.companyA)
		var count int
		mustScanP3(t, f.db, `SELECT count(*) FROM source_files WHERE company_id=$1 AND marketplace_key='amazon'`, []any{f.companyA}, &count)
		if count != 0 {
			t.Fatalf("denied Amazon sources=%d", count)
		}
	})
	t.Run("label permission is required before persistence", func(t *testing.T) {
		var roleID string
		mustScanP3(t, f.db, `SELECT id FROM roles WHERE company_id=$1 AND name='Flipkart Operator'`, []any{f.companyA}, &roleID)
		mustExecP3(t, f.db, `DELETE FROM role_permissions WHERE company_id=$1 AND role_id=$2 AND permission_key='labels.upload'`, f.companyA, roleID)
		pdf := f.register("amazon-permission-denied", pdfextractor.Page{Number: 1, Text: amazonText("406-1100000-1100000", "TRACKDENIED2", "AMZ-KNOWN", "1")})
		if _, uploadErr := service.Upload(ctx, f.principalA, "permission-denied.pdf", pdf); !errors.Is(uploadErr, authorization.ErrPermissionDenied) {
			t.Fatalf("upload error=%v", uploadErr)
		}
		mustExecP3(t, f.db, `INSERT INTO role_permissions(company_id,role_id,permission_key) VALUES($1,$2,'labels.upload')`, f.companyA, roleID)
		var count int
		mustScanP3(t, f.db, `SELECT count(*) FROM source_files WHERE company_id=$1 AND marketplace_key='amazon'`, []any{f.companyA}, &count)
		if count != 0 {
			t.Fatalf("permission-denied Amazon sources=%d", count)
		}
	})

	known := f.register("amazon-known", pdfextractor.Page{Number: 6, Text: amazonText("406-2000000-2000000", "TRACKKNOWN1", "AMZ-KNOWN", "2")})
	uploaded, err := service.Upload(ctx, f.principalA, "amazon-known.pdf", known)
	if err != nil || uploaded.Job.ParserVersion != amazon.ParserVersion {
		t.Fatalf("upload=%#v err=%v", uploaded, err)
	}
	if _, claimed, claimErr := f.service.claim(ctx); claimErr != nil || claimed {
		t.Fatalf("Flipkart worker claimed Amazon job: claimed=%v err=%v", claimed, claimErr)
	}
	processed, err := service.processNext()
	if err != nil || !processed {
		t.Fatalf("processed=%v err=%v", processed, err)
	}
	details, err := service.Get(ctx, f.principalA, uploaded.Job.ID)
	if err != nil || details.Job.Status != "processed" || details.Job.ParserVersion != amazon.ParserVersion || len(details.Orders) != 1 || details.Orders[0].SourcePage != 6 || len(details.Orders[0].Documents) != 1 || details.Orders[0].Documents[0].SourcePage != 6 || details.Orders[0].Documents[0].Role != "shipping_label" || details.Orders[0].Items[0].ProductID == nil || *details.Orders[0].Items[0].ProductID != f.productID || details.Orders[0].Items[0].Quantity == nil || *details.Orders[0].Items[0].Quantity != 2 {
		t.Fatalf("details=%#v err=%v", details, err)
	}
	var traceable bool
	mustScanP3(t, f.db, `SELECT o.source_file_id=j.source_file_id AND o.parser_version=j.parser_version AND f.marketplace_key='amazon' AND f.storage_key LIKE $3 FROM marketplace_orders o JOIN processing_jobs j ON j.company_id=o.company_id AND j.id=o.processing_job_id JOIN source_files f ON f.company_id=o.company_id AND f.id=o.source_file_id WHERE o.company_id=$1 AND o.processing_job_id=$2`, []any{f.companyA, uploaded.Job.ID, f.companyA + "/%"}, &traceable)
	if !traceable {
		t.Fatal("Amazon source/job/order/parser traceability was not preserved")
	}

	t.Run("OCR label and invoice associate by order ID with both pages traceable", func(t *testing.T) {
		orderID := "406-2500000-2500000"
		pdf := f.register("amazon-associated",
			pdfextractor.Page{Number: 5, ExtractionMethod: "ocr", Text: "amazon.in Shipping Label\nOrder ID: " + orderID + "\nAWB: TRACKASSOC250"},
			pdfextractor.Page{Number: 11, ExtractionMethod: "text", Text: "amazon.in\nTax Invoice\nOrder Number: " + orderID + "\nSeller SKU: AMZ-KNOWN\nQuantity: 3"})
		result, uploadErr := service.Upload(ctx, f.principalA, "associated.pdf", pdf)
		if uploadErr != nil {
			t.Fatal(uploadErr)
		}
		if ok, processErr := service.processNext(); processErr != nil || !ok {
			t.Fatalf("process=%v err=%v", ok, processErr)
		}
		detail, getErr := service.Get(ctx, f.principalA, result.Job.ID)
		if getErr != nil || detail.Job.Status != "processed" || len(detail.Orders) != 1 || detail.Orders[0].SourcePage != 5 || len(detail.Orders[0].Documents) != 2 || detail.Orders[0].Documents[0].ExtractionMethod != "ocr" || detail.Orders[0].Documents[1].Role != "invoice" || detail.Orders[0].Items[0].Quantity == nil || *detail.Orders[0].Items[0].Quantity != 3 {
			t.Fatalf("detail=%#v err=%v", detail, getErr)
		}
	})
	if _, err = service.Get(ctx, f.principalB, uploaded.Job.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant get=%v", err)
	}
	duplicate, err := service.Upload(ctx, f.principalA, "same-source.pdf", known)
	if err != nil || !duplicate.DuplicateSource || duplicate.Job.ID != uploaded.Job.ID {
		t.Fatalf("duplicate=%#v err=%v", duplicate, err)
	}

	t.Run("unknown SKU and missing quantity remain review values", func(t *testing.T) {
		pdf := f.register("amazon-review", pdfextractor.Page{Number: 9, Text: amazonText("406-3000000-3000000", "TRACKREVIEW1", "AMZ-UNKNOWN", "")})
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
		if review.Job.Status != "needs_review" || review.Orders[0].SourcePage != 9 || item.ProductID != nil || item.Quantity != nil || item.QuantitySource != "missing" || item.ResolutionStatus != "unresolved" {
			t.Fatalf("review=%#v", review)
		}
	})

	t.Run("retry uses newly trained exact Amazon SKU mapping", func(t *testing.T) {
		pdf := f.register("amazon-retry", pdfextractor.Page{Number: 3, Text: amazonText("406-4000000-4000000", "TRACKRETRY1", "AMZ-RETRY", "4")})
		result, uploadErr := service.Upload(ctx, f.principalA, "retry.pdf", pdf)
		if uploadErr != nil {
			t.Fatal(uploadErr)
		}
		if _, processErr := service.processNext(); processErr != nil {
			t.Fatal(processErr)
		}
		mustExecP3(t, f.db, `INSERT INTO sku_mappings(company_id,marketplace_key,product_id,sku) VALUES($1,'amazon',$2,'AMZ-RETRY')`, f.companyA, f.productID)
		job, retryErr := service.Retry(ctx, f.principalA, result.Job.ID)
		if retryErr != nil || job.Status != "queued" || job.ParserVersion != amazon.ParserVersion {
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

	t.Run("duplicate Amazon business identifier is visible", func(t *testing.T) {
		pdf := f.register("amazon-business-duplicate", pdfextractor.Page{Number: 2, Text: amazonText("406-2000000-2000000", "TRACKOTHER1", "AMZ-KNOWN", "1")})
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

func TestAmazonProcessingMigrationUpDown(t *testing.T) {
	f := setupPhaseThree(t)
	ctx := context.Background()
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	schema := "p7_amazon_" + fmt.Sprint(time.Now().UnixNano())
	if _, err = tx.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SET LOCAL search_path TO `+schema+`,public`); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("..", "..", "migrations")
	for _, name := range []string{"000001_core_platform.up.sql", "000002_tenant_sessions.up.sql", "000003_product_master.up.sql", "000004_flipkart_processing.up.sql", "000005_flipkart_worker_leases.up.sql", "000006_batch_foundation.up.sql", "000007_print_generation.up.sql", "000008_worker_assignments_reprints.up.sql", "000009_inventory_ledger.up.sql", "000010_inventory_outbound_reservations.up.sql", "000011_dashboard_reporting.up.sql", "000012_amazon_processing.up.sql", "000013_amazon_document_association.up.sql"} {
		data, readErr := os.ReadFile(filepath.Join(root, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = tx.Exec(ctx, string(data)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	var exists *string
	if err = tx.QueryRow(ctx, `SELECT to_regclass($1)`, schema+".marketplace_order_documents").Scan(&exists); err != nil || exists == nil {
		t.Fatalf("up=%v err=%v", exists, err)
	}
	down, err := os.ReadFile(filepath.Join(root, "000013_amazon_document_association.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	exists = nil
	if err = tx.QueryRow(ctx, `SELECT to_regclass($1)`, schema+".marketplace_order_documents").Scan(&exists); err != nil || exists != nil {
		t.Fatalf("down=%v err=%v", exists, err)
	}
}

func amazonText(orderID, awb, sku, quantity string) string {
	text := "amazon.in\nOrder ID: " + orderID + "\nAWB: " + awb + "\nSeller SKU: " + sku
	if quantity != "" {
		text += "\nQuantity: " + quantity
	}
	return text
}
