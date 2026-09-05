// This file contains PostgreSQL-backed tests for cross-layer behavior, tenant isolation, and domain invariants in the marketplace orchestration package.
package marketplace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/marketplace/myntra"
)

func TestMyntraBatchAPostgreSQLIntegration(t *testing.T) {
	f := setupPhaseThree(t)
	ctx := context.Background()
	mustExecP3(t, f.db, `INSERT INTO module_entitlements(company_id,module_key,enabled) VALUES($1,'myntra',true),($2,'myntra',true)`, f.companyA, f.companyB)
	mustExecP3(t, f.db, `INSERT INTO sku_mappings(company_id,marketplace_key,product_id,sku) VALUES($1,'myntra',$2,'SANITIZED-SKU_01')`, f.companyA, f.productID)
	service, err := newServiceForProcessor(f.db, authorization.NewService(f.db), f.service.storage, nil, myntraProcessor())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("myntra", "testdata", "sanitized_packed_orders.csv"))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("authorization precedes persistence", func(t *testing.T) {
		mustExecP3(t, f.db, `UPDATE module_entitlements SET enabled=false WHERE company_id=$1 AND module_key='myntra'`, f.companyA)
		if _, uploadErr := service.UploadWithIdempotency(ctx, f.principalA, "orders.csv", data, "denied-import"); !errors.Is(uploadErr, authorization.ErrModuleUnavailable) {
			t.Fatalf("upload error=%v", uploadErr)
		}
		mustExecP3(t, f.db, `UPDATE module_entitlements SET enabled=true WHERE company_id=$1 AND module_key='myntra'`, f.companyA)
		var count int
		mustScanP3(t, f.db, `SELECT count(*) FROM source_files WHERE company_id=$1 AND marketplace_key='myntra'`, []any{f.companyA}, &count)
		if count != 0 {
			t.Fatalf("denied sources=%d", count)
		}
		var roleID string
		mustScanP3(t, f.db, `SELECT id FROM roles WHERE company_id=$1 AND name='Flipkart Operator'`, []any{f.companyA}, &roleID)
		mustExecP3(t, f.db, `DELETE FROM role_permissions WHERE company_id=$1 AND role_id=$2 AND permission_key='labels.process'`, f.companyA, roleID)
		if _, uploadErr := service.UploadWithIdempotency(ctx, f.principalA, "orders.csv", data, "permission-denied-import"); !errors.Is(uploadErr, authorization.ErrPermissionDenied) {
			t.Fatalf("permission error=%v", uploadErr)
		}
		mustExecP3(t, f.db, `INSERT INTO role_permissions(company_id,role_id,permission_key) VALUES($1,$2,'labels.process')`, f.companyA, roleID)
		mustScanP3(t, f.db, `SELECT count(*) FROM source_files WHERE company_id=$1 AND marketplace_key='myntra'`, []any{f.companyA}, &count)
		if count != 0 {
			t.Fatalf("permission-denied sources=%d", count)
		}
	})

	var inventoryBefore int
	mustScanP3(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, []any{f.companyA}, &inventoryBefore)
	uploaded, err := service.UploadWithIdempotency(ctx, f.principalA, "packed-orders.csv", data, "myntra-import-1")
	if err != nil || uploaded.Job.ParserVersion != myntra.ParserVersion {
		t.Fatalf("upload=%#v err=%v", uploaded, err)
	}
	if processed, processErr := service.processNext(); processErr != nil || !processed {
		t.Fatalf("processed=%v err=%v", processed, processErr)
	}
	details, err := service.Get(ctx, f.principalA, uploaded.Job.ID)
	if err != nil || details.Job.Status != "needs_review" || details.Job.TotalPages != 3 || len(details.Orders) != 3 {
		t.Fatalf("details=%#v err=%v", details, err)
	}
	first := details.Orders[0]
	if first.SourcePage != 2 || first.MarketplaceOrderID == nil || *first.MarketplaceOrderID != "7000000001" || first.AWB == nil || *first.AWB != "MYSP1000000001" || first.Items[0].RawSKU == nil || *first.Items[0].RawSKU != "SANITIZED-SKU_01" || first.Items[0].ProductID == nil || first.Items[0].Quantity != nil || first.Items[0].QuantitySource != "missing" {
		t.Fatalf("first=%#v", first)
	}
	var metadata struct {
		MyntraSKUCode     string `json:"myntra_sku_code"`
		StorePacketID     string `json:"store_packet_id"`
		OrderReleaseID    string `json:"order_release_id"`
		MarketplaceStatus string `json:"marketplace_status"`
		SourceKind        string `json:"source_kind"`
		Extractor         string `json:"extractor"`
	}
	if err = json.Unmarshal(first.Metadata, &metadata); err != nil {
		t.Fatalf("decode metadata %s: %v", first.Metadata, err)
	}
	if metadata.MyntraSKUCode != "MYNTRASKU100000001" ||
		metadata.StorePacketID != "2000000000001" ||
		metadata.OrderReleaseID != "300000000001" ||
		metadata.MarketplaceStatus != "PICKED" ||
		metadata.SourceKind != "csv" || metadata.Extractor != "csv" {
		t.Fatalf("metadata=%#v", metadata)
	}
	if _, getErr := service.Get(ctx, f.principalB, uploaded.Job.ID); !errors.Is(getErr, ErrNotFound) {
		t.Fatalf("cross-tenant get=%v", getErr)
	}
	replay, err := service.UploadWithIdempotency(ctx, f.principalA, "packed-orders.csv", data, "myntra-import-1")
	if err != nil || !replay.DuplicateSource || replay.Job.ID != uploaded.Job.ID {
		t.Fatalf("replay=%#v err=%v", replay, err)
	}
	changed := append([]byte{}, data...)
	changed = append(changed, '\n')
	if _, conflictErr := service.UploadWithIdempotency(ctx, f.principalA, "packed-orders.csv", changed, "myntra-import-1"); !errors.Is(conflictErr, ErrIdempotencyConflict) {
		t.Fatalf("conflict=%v", conflictErr)
	}
	var inventoryAfter int
	mustScanP3(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, []any{f.companyA}, &inventoryAfter)
	if inventoryAfter != inventoryBefore {
		t.Fatalf("inventory changed from %d to %d", inventoryBefore, inventoryAfter)
	}
	var eligible int
	mustScanP3(t, f.db, `SELECT count(*) FROM marketplace_orders o JOIN marketplace_order_items i ON i.company_id=o.company_id AND i.order_id=o.id WHERE o.company_id=$1 AND o.marketplace_key='myntra' AND o.status='resolved' AND i.quantity IS NOT NULL`, []any{f.companyA}, &eligible)
	if eligible != 0 {
		t.Fatalf("quantity-dependent eligible rows=%d", eligible)
	}
}

func TestMyntraProcessingMigrationUpDown(t *testing.T) {
	f := setupPhaseThree(t)
	ctx := context.Background()
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	root := filepath.Join("..", "..", "migrations")
	down, err := os.ReadFile(filepath.Join(root, "000019_myntra_csv_processing.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	var exists *string
	if err = tx.QueryRow(ctx, `SELECT to_regclass('processing_jobs_myntra_claim_idx')`).Scan(&exists); err != nil || exists != nil {
		t.Fatalf("down=%v err=%v", exists, err)
	}
	up, err := os.ReadFile(filepath.Join(root, "000019_myntra_csv_processing.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(up)); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `SELECT to_regclass('processing_jobs_myntra_claim_idx')`).Scan(&exists); err != nil || exists == nil {
		t.Fatalf("up=%v err=%v", exists, err)
	}
}
