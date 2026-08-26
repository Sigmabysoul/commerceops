package product

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProductMasterPostgreSQL(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	suffix := fmt.Sprint(time.Now().UnixNano())
	var companyOne, companyTwo, managerID, viewerID, roleID string
	mustScan(t, db, `INSERT INTO companies(name) VALUES($1) RETURNING id`, []any{"Products A " + suffix}, &companyOne)
	mustScan(t, db, `INSERT INTO companies(name) VALUES($1) RETURNING id`, []any{"Products B " + suffix}, &companyTwo)
	mustScan(t, db, `INSERT INTO users(email,password_hash) VALUES($1,'test-hash') RETURNING id`, []any{"product-manager-" + suffix + "@example.test"}, &managerID)
	mustScan(t, db, `INSERT INTO users(email,password_hash) VALUES($1,'test-hash') RETURNING id`, []any{"product-viewer-" + suffix + "@example.test"}, &viewerID)
	defer cleanup(t, db, companyOne, companyTwo, managerID, viewerID)
	mustExec(t, db, `INSERT INTO company_users(company_id,user_id) VALUES($1,$2),($1,$3),($4,$2)`, companyOne, managerID, viewerID, companyTwo)
	mustScan(t, db, `INSERT INTO roles(company_id,name) VALUES($1,'Product Manager') RETURNING id`, []any{companyOne}, &roleID)
	mustExec(t, db, `INSERT INTO role_permissions(company_id,role_id,permission_key) VALUES($1,$2,'products.view'),($1,$2,'products.manage')`, companyOne, roleID)
	mustExec(t, db, `INSERT INTO company_user_roles(company_id,user_id,role_id) VALUES($1,$2,$3)`, companyOne, managerID, roleID)

	service := NewService(db, authorization.NewService(db))
	manager := auth.Principal{CompanyID: companyOne, UserID: managerID}
	viewer := auth.Principal{CompanyID: companyOne, UserID: viewerID}

	t.Run("permission denied", func(t *testing.T) {
		if _, err := service.ListProducts(ctx, viewer, "", ""); !errors.Is(err, authorization.ErrPermissionDenied) {
			t.Fatalf("expected denial, got %v", err)
		}
		if _, err := service.CreateProduct(ctx, viewer, ProductInput{InternalCode: "DENIED", Name: "Denied"}); !errors.Is(err, authorization.ErrPermissionDenied) {
			t.Fatalf("expected denial, got %v", err)
		}
	})

	productOne, err := service.CreateProduct(ctx, manager, ProductInput{InternalCode: "GB-AVX-3B", Name: "Garbage Bag", Brand: str("Averx"), Variant: str("3 Bag"), UnitCount: intp(3)})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	t.Run("internal code uniqueness and tenant isolation", func(t *testing.T) {
		if _, err := service.CreateProduct(ctx, manager, ProductInput{InternalCode: "GB-AVX-3B", Name: "Duplicate"}); !errors.Is(err, ErrConflict) {
			t.Fatalf("expected conflict, got %v", err)
		}
		var otherProduct string
		mustScan(t, db, `INSERT INTO products(company_id,internal_code,name) VALUES($1,'GB-AVX-3B','Other Tenant') RETURNING id`, []any{companyTwo}, &otherProduct)
		items, err := service.ListProducts(ctx, manager, "", "")
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 1 || items[0].ID != productOne.ID {
			t.Fatalf("cross-tenant products visible: %#v", items)
		}
		if _, err := service.CreateMapping(ctx, manager, MappingInput{MarketplaceKey: "flipkart", ProductID: otherProduct, SKU: "CROSS-TENANT"}); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("cross-tenant mapping result: %v", err)
		}
	})

	mapping, err := service.CreateMapping(ctx, manager, MappingInput{MarketplaceKey: "flipkart", ProductID: productOne.ID, SKU: "  ABC-XYZ-123  ", QuantityMultiplier: 2, InterpretationMetadata: map[string]any{"note": "two packs"}})
	if err != nil {
		t.Fatalf("create mapping: %v", err)
	}
	if mapping.SKU != "ABC-XYZ-123" {
		t.Fatalf("SKU not trimmed: %q", mapping.SKU)
	}

	t.Run("deterministic resolution", func(t *testing.T) {
		resolved, err := service.Resolve(ctx, manager, "flipkart", "ABC-XYZ-123")
		if err != nil {
			t.Fatal(err)
		}
		if resolved.Status != "resolved" || resolved.Product == nil || resolved.Product.ID != productOne.ID || resolved.Mapping == nil || resolved.Mapping.QuantityMultiplier != 2 {
			t.Fatalf("unexpected resolution: %#v", resolved)
		}
		for _, request := range [][2]string{{"amazon", "ABC-XYZ-123"}, {"flipkart", "abc-xyz-123"}, {"flipkart", "unknown"}} {
			result, err := service.Resolve(ctx, manager, request[0], request[1])
			if err != nil || result.Status != "unresolved" || result.Product != nil {
				t.Fatalf("expected unresolved for %#v: %#v %v", request, result, err)
			}
		}
	})

	t.Run("ambiguous active mapping is rejected", func(t *testing.T) {
		if _, err := service.CreateMapping(ctx, manager, MappingInput{MarketplaceKey: "flipkart", ProductID: productOne.ID, SKU: "ABC-XYZ-123"}); !errors.Is(err, ErrConflict) {
			t.Fatalf("expected conflict, got %v", err)
		}
	})

	t.Run("inactive mapping and product do not resolve", func(t *testing.T) {
		inactive, err := service.UpdateMapping(ctx, manager, mapping.ID, MappingInput{MarketplaceKey: mapping.MarketplaceKey, ProductID: mapping.ProductID, SKU: mapping.SKU, QuantityMultiplier: mapping.QuantityMultiplier, InterpretationMetadata: map[string]any{}, Status: "inactive"})
		if err != nil || inactive.Status != "inactive" {
			t.Fatalf("deactivate mapping: %#v %v", inactive, err)
		}
		result, err := service.Resolve(ctx, manager, "flipkart", mapping.SKU)
		if err != nil || result.Status != "unresolved" {
			t.Fatalf("inactive mapping resolved: %#v %v", result, err)
		}
		mapping, err = service.CreateMapping(ctx, manager, MappingInput{MarketplaceKey: "flipkart", ProductID: productOne.ID, SKU: mapping.SKU})
		if err != nil {
			t.Fatalf("replace inactive mapping: %v", err)
		}
		productOne, err = service.UpdateProduct(ctx, manager, productOne.ID, ProductInput{InternalCode: productOne.InternalCode, Name: productOne.Name, Brand: productOne.Brand, Variant: productOne.Variant, UnitCount: productOne.UnitCount, Status: "inactive"})
		if err != nil {
			t.Fatalf("deactivate product: %v", err)
		}
		result, err = service.Resolve(ctx, manager, "flipkart", mapping.SKU)
		if err != nil || result.Status != "unresolved" {
			t.Fatalf("inactive product resolved: %#v %v", result, err)
		}
	})

	t.Run("same SKU is independent across companies", func(t *testing.T) {
		var productTwo string
		mustScan(t, db, `INSERT INTO products(company_id,internal_code,name) VALUES($1,'OTHER','Other') RETURNING id`, []any{companyTwo}, &productTwo)
		mustExec(t, db, `INSERT INTO sku_mappings(company_id,marketplace_key,product_id,sku) VALUES($1,'flipkart',$2,'ABC-XYZ-123')`, companyTwo, productTwo)
		var count int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM sku_mappings WHERE sku='ABC-XYZ-123'`).Scan(&count); err != nil || count != 3 {
			t.Fatalf("independent mappings count=%d err=%v", count, err)
		}
	})

	t.Run("changes are audited", func(t *testing.T) {
		var count int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE company_id=$1 AND actor_user_id=$2 AND target_type IN ('product','sku_mapping')`, companyOne, managerID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count < 5 {
			t.Fatalf("expected product audit records, got %d", count)
		}
		var otherCount int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE company_id=$1 AND target_type IN ('product','sku_mapping')`, companyTwo).Scan(&otherCount); err != nil || otherCount != 0 {
			t.Fatalf("cross-tenant audit records: %d %v", otherCount, err)
		}
	})
}

func mustScan(t *testing.T, db *pgxpool.Pool, query string, args []any, destinations ...any) {
	t.Helper()
	if err := db.QueryRow(context.Background(), query, args...).Scan(destinations...); err != nil {
		t.Fatalf("fixture query: %v", err)
	}
}
func mustExec(t *testing.T, db *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("fixture command: %v", err)
	}
}
func cleanup(t *testing.T, db *pgxpool.Pool, companyOne, companyTwo string, userIDs ...string) {
	t.Helper()
	ctx := context.Background()
	companies := []string{companyOne, companyTwo}
	for _, table := range []string{"sku_mappings", "products", "audit_logs", "sessions", "module_entitlements", "company_user_roles", "role_permissions", "employees", "roles", "company_users"} {
		if _, err := db.Exec(ctx, "DELETE FROM "+table+" WHERE company_id=ANY($1::uuid[])", companies); err != nil {
			t.Errorf("cleanup %s: %v", table, err)
		}
	}
	if _, err := db.Exec(ctx, `DELETE FROM users WHERE id=ANY($1::uuid[])`, userIDs); err != nil {
		t.Errorf("cleanup users: %v", err)
	}
	if _, err := db.Exec(ctx, `DELETE FROM companies WHERE id=ANY($1::uuid[])`, companies); err != nil {
		t.Errorf("cleanup companies: %v", err)
	}
}
func str(v string) *string { return &v }
func intp(v int) *int      { return &v }
