// This file contains PostgreSQL-backed tests for cross-layer behavior, tenant isolation, and domain invariants in the marketplace orchestration package.
package marketplace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/marketplace/snapdeal"
	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
)

func TestSnapdealPostgreSQLIntegration(t *testing.T) {
	f := setupPhaseThree(t)
	ctx := context.Background()
	mustExecP3(t, f.db, `INSERT INTO module_entitlements(company_id,module_key,enabled) VALUES($1,'snapdeal',true),($2,'snapdeal',true)`, f.companyA, f.companyB)
	mustExecP3(t, f.db, `INSERT INTO sku_mappings(company_id,marketplace_key,product_id,sku) VALUES($1,'snapdeal',$2,'9_SAFE-SKU-R1')`, f.companyA, f.productID)
	service, err := newServiceForProcessor(f.db, authorization.NewService(f.db), f.service.storage, f.extractor, snapdealProcessor())
	if err != nil {
		t.Fatal(err)
	}
	t.Run("entitlement and permission", func(t *testing.T) {
		pdf := f.register("snap-denied", pdfextractor.Page{Number: 1, Text: snapShipping("88000000011", "SF0000000011DM", "1")}, pdfextractor.Page{Number: 2, Text: snapInvoice("88000000011", "9_SAFE-SKU-R1", "1")})
		mustExecP3(t, f.db, `UPDATE module_entitlements SET enabled=false WHERE company_id=$1 AND module_key='snapdeal'`, f.companyA)
		if _, e := service.Upload(ctx, f.principalA, "denied.pdf", pdf); !errors.Is(e, authorization.ErrModuleUnavailable) {
			t.Fatalf("err=%v", e)
		}
		mustExecP3(t, f.db, `UPDATE module_entitlements SET enabled=true WHERE company_id=$1 AND module_key='snapdeal'`, f.companyA)
		var role string
		mustScanP3(t, f.db, `SELECT id FROM roles WHERE company_id=$1 AND name='Flipkart Operator'`, []any{f.companyA}, &role)
		mustExecP3(t, f.db, `DELETE FROM role_permissions WHERE company_id=$1 AND role_id=$2 AND permission_key='labels.process'`, f.companyA, role)
		if _, e := service.Upload(ctx, f.principalA, "denied.pdf", pdf); !errors.Is(e, authorization.ErrPermissionDenied) {
			t.Fatalf("permission err=%v", e)
		}
		mustExecP3(t, f.db, `INSERT INTO role_permissions(company_id,role_id,permission_key) VALUES($1,$2,'labels.process')`, f.companyA, role)
		var count int
		mustScanP3(t, f.db, `SELECT count(*) FROM source_files WHERE company_id=$1 AND marketplace_key='snapdeal'`, []any{f.companyA}, &count)
		if count != 0 {
			t.Fatalf("denied sources=%d", count)
		}
	})
	pdf := f.register("snap-known", pdfextractor.Page{Number: 1, ExtractionMethod: "text", Text: snapShipping("88000000012", "SF0000000012DM", "2")}, pdfextractor.Page{Number: 2, ExtractionMethod: "text", Text: snapInvoice("88000000012", "9_SAFE-SKU-R1", "2")})
	var before int
	mustScanP3(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, []any{f.companyA}, &before)
	uploaded, err := service.Upload(ctx, f.principalA, "snapdeal.pdf", pdf)
	if err != nil || uploaded.Job.ParserVersion != snapdeal.ParserVersion {
		t.Fatalf("upload=%#v err=%v", uploaded, err)
	}
	if ok, e := service.processNext(); e != nil || !ok {
		t.Fatalf("process=%t err=%v", ok, e)
	}
	details, err := service.Get(ctx, f.principalA, uploaded.Job.ID)
	if err != nil || details.Job.Status != "processed" || len(details.Orders) != 1 || len(details.Orders[0].Documents) != 2 || details.Orders[0].Items[0].ProductID == nil || details.Orders[0].Items[0].Quantity == nil || *details.Orders[0].Items[0].Quantity != 2 {
		t.Fatalf("details=%#v err=%v", details, err)
	}
	if _, e := service.Get(ctx, f.principalB, uploaded.Job.ID); !errors.Is(e, ErrNotFound) {
		t.Fatalf("cross tenant=%v", e)
	}
	duplicate, err := service.Upload(ctx, f.principalA, "same.pdf", pdf)
	if err != nil || !duplicate.DuplicateSource || duplicate.Job.ID != uploaded.Job.ID {
		t.Fatalf("duplicate=%#v err=%v", duplicate, err)
	}
	var after int
	mustScanP3(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, []any{f.companyA}, &after)
	if after != before {
		t.Fatalf("inventory %d -> %d", before, after)
	}
	t.Run("unknown and conflicting evidence reviews", func(t *testing.T) {
		reviewPDF := f.register("snap-review", pdfextractor.Page{Number: 1, Text: snapShipping("88000000013", "SF0000000013DM", "1")}, pdfextractor.Page{Number: 2, Text: snapInvoice("88000000013", "9_UNKNOWN", "2")})
		result, e := service.Upload(ctx, f.principalA, "review.pdf", reviewPDF)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = service.processNext(); e != nil {
			t.Fatal(e)
		}
		review, e := service.Get(ctx, f.principalA, result.Job.ID)
		if e != nil || review.Job.Status != "needs_review" || review.Orders[0].Items[0].ProductID != nil || review.Orders[0].Items[0].Quantity != nil {
			t.Fatalf("review=%#v err=%v", review, e)
		}
	})
}
func TestSnapdealMigrationUpDown(t *testing.T) {
	f := setupPhaseThree(t)
	ctx := context.Background()
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	root := filepath.Join("..", "..", "migrations")
	down, err := os.ReadFile(filepath.Join(root, "000020_snapdeal_processing.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	var exists *string
	if err = tx.QueryRow(ctx, `SELECT to_regclass('processing_jobs_snapdeal_claim_idx')`).Scan(&exists); err != nil || exists != nil {
		t.Fatalf("down=%v err=%v", exists, err)
	}
	up, err := os.ReadFile(filepath.Join(root, "000020_snapdeal_processing.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(up)); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `SELECT to_regclass('processing_jobs_snapdeal_claim_idx')`).Scan(&exists); err != nil || exists == nil {
		t.Fatalf("up=%v err=%v", exists, err)
	}
}
func snapShipping(order, awb, qty string) string {
	return "snapdeal\nSHADOWFAX\n" + awb + "\nDELIVERY ADDRESS\nSafe\nSUBORDER CODE SELLER GSTIN QUANTITY\n" + order + " |\nSAFE SELLER 19SAFE0000A1AA " + qty + "\n9_SAFESKUR1\nSnapdeal Reference No\nSHIPPED FROM\nSafe"
}
func snapInvoice(order, sku, qty string) string {
	return "TAX INVOICE\nINVOICE NUMBER : SAFE/1\nITEM DESCRIPTION QTY RATE TOTAL DISC TAXABLE VALUE\nSafe\n" + qty + " 100 100 0 100\nSKU CODE: " + sku + "\nSUBORDER : " + order + "\nHSN: 0000"
}
