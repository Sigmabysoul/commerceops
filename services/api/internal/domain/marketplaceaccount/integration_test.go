// This file verifies seller-account authorization, tenant isolation, and the separation from device identity.
package marketplaceaccount

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/platform/auth"
	"github.com/commerceops/commerceops/services/api/internal/platform/authorization"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSellerAccountsArePermissionAndCompanyScoped(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" { t.Skip("TEST_DATABASE_URL is not set") }
	ctx := context.Background()
	db, err := pgxpool.New(ctx, url)
	if err != nil { t.Fatalf("open database: %v", err) }
	defer db.Close()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var companyA, companyB, userID, roleID string
	mustAccountScan(t, db, `INSERT INTO companies(name) VALUES($1) RETURNING id`, []any{"Seller account A " + suffix}, &companyA)
	mustAccountScan(t, db, `INSERT INTO companies(name) VALUES($1) RETURNING id`, []any{"Seller account B " + suffix}, &companyB)
	hash, err := auth.HashPassword("seller-account-test-password")
	if err != nil { t.Fatalf("hash password: %v", err) }
	mustAccountScan(t, db, `INSERT INTO users(email,password_hash) VALUES($1,$2) RETURNING id`, []any{"seller-" + suffix + "@example.test", hash}, &userID)
	defer func() {
		for _, table := range []string{"audit_logs", "company_user_roles", "role_permissions", "roles", "company_users", "marketplace_accounts", "business_identities"} {
			_, _ = db.Exec(ctx, "DELETE FROM "+table+" WHERE company_id=ANY($1::uuid[])", []string{companyA, companyB})
		}
		_, _ = db.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID)
		_, _ = db.Exec(ctx, `DELETE FROM companies WHERE id=ANY($1::uuid[])`, []string{companyA, companyB})
	}()
	mustAccountExec(t, db, `INSERT INTO company_users(company_id,user_id) VALUES($1,$2),($3,$2)`, companyA, userID, companyB)
	mustAccountScan(t, db, `INSERT INTO roles(company_id,name) VALUES($1,$2) RETURNING id`, []any{companyA, "Seller Accounts " + suffix}, &roleID)
	mustAccountExec(t, db, `INSERT INTO role_permissions(company_id,role_id,permission_key) VALUES($1,$2,'marketplace_accounts.view'),($1,$2,'marketplace_accounts.manage')`, companyA, roleID)
	mustAccountExec(t, db, `INSERT INTO company_user_roles(company_id,user_id,role_id) VALUES($1,$2,$3)`, companyA, userID, roleID)

	service := NewService(db, authorization.NewService(db))
	principalA := auth.Principal{UserID: userID, CompanyID: companyA}
	identity, err := service.SaveIdentity(ctx, principalA, "", IdentityInput{DisplayName: "Brothers " + suffix, Status: "active"})
	if err != nil { t.Fatalf("save identity: %v", err) }
	account, err := service.SaveAccount(ctx, principalA, "", AccountInput{MarketplaceKey: "flipkart", BusinessIdentityID: identity.ID, InternalKey: "brothers_" + suffix, DisplayName: "Brothers Flipkart", IntegrationMode: "manual_upload", Status: "active"})
	if err != nil { t.Fatalf("save account: %v", err) }
	if account.IdentityName != identity.DisplayName { t.Fatalf("identity name = %q, want %q", account.IdentityName, identity.DisplayName) }
	if _, err = service.Accounts(ctx, auth.Principal{UserID: userID, CompanyID: companyB}, ""); !errors.Is(err, authorization.ErrPermissionDenied) { t.Fatalf("other company access result: %v", err) }
	if _, err = service.SaveAccount(ctx, principalA, "", AccountInput{MarketplaceKey: "flipkart", BusinessIdentityID: identity.ID, InternalKey: account.InternalKey, DisplayName: "Duplicate", IntegrationMode: "manual_upload", Status: "active"}); !errors.Is(err, ErrConflict) { t.Fatalf("duplicate account key result: %v", err) }
}

func mustAccountScan(t *testing.T, db *pgxpool.Pool, query string, args []any, destination ...any) { t.Helper(); if err := db.QueryRow(context.Background(), query, args...).Scan(destination...); err != nil { t.Fatalf("fixture query: %v", err) } }
func mustAccountExec(t *testing.T, db *pgxpool.Pool, query string, args ...any) { t.Helper(); if _, err := db.Exec(context.Background(), query, args...); err != nil { t.Fatalf("fixture command: %v", err) } }
