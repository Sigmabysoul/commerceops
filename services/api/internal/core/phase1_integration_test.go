package core

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

func TestPhaseOneSecurityAndTenantBehavior(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	password := "phase-one-test-password"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	var companyOne, companyTwo, adminID, disabledID, limitedID, adminRole, limitedRole string
	mustScan(t, db, `INSERT INTO companies (name) VALUES ($1) RETURNING id`, []any{"Phase One A " + suffix}, &companyOne)
	mustScan(t, db, `INSERT INTO companies (name) VALUES ($1) RETURNING id`, []any{"Phase One B " + suffix}, &companyTwo)
	mustScan(t, db, `INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id`, []any{"admin-" + suffix + "@example.test", hash}, &adminID)
	mustScan(t, db, `INSERT INTO users (email, password_hash, status) VALUES ($1, $2, 'disabled') RETURNING id`, []any{"disabled-" + suffix + "@example.test", hash}, &disabledID)
	mustScan(t, db, `INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id`, []any{"limited-" + suffix + "@example.test", hash}, &limitedID)
	defer cleanupFixture(t, db, companyOne, companyTwo, adminID, disabledID, limitedID)

	mustExec(t, db, `INSERT INTO company_users (company_id, user_id) VALUES ($1,$2),($1,$3),($1,$4),($5,$2)`, companyOne, adminID, disabledID, limitedID, companyTwo)
	mustScan(t, db, `INSERT INTO roles (company_id, name) VALUES ($1, 'Administrator') RETURNING id`, []any{companyOne}, &adminRole)
	mustScan(t, db, `INSERT INTO roles (company_id, name) VALUES ($1, 'Limited') RETURNING id`, []any{companyOne}, &limitedRole)
	mustExec(t, db, `INSERT INTO role_permissions (company_id, role_id, permission_key) SELECT $1,$2,key FROM permissions`, companyOne, adminRole)
	mustExec(t, db, `INSERT INTO company_user_roles (company_id,user_id,role_id) VALUES ($1,$2,$3),($1,$4,$5)`, companyOne, adminID, adminRole, limitedID, limitedRole)
	mustExec(t, db, `INSERT INTO employees (company_id,display_name) VALUES ($1,'Tenant A Employee'),($2,'Tenant B Employee')`, companyOne, companyTwo)

	authService := auth.NewService(db, time.Hour)
	authorizer := authorization.NewService(db)
	service := NewService(db, authorizer)

	t.Run("successful login and session tenant", func(t *testing.T) {
		token, principal, err := authService.Login(ctx, "ADMIN-"+suffix+"@EXAMPLE.TEST", password, companyOne)
		if err != nil {
			t.Fatalf("login: %v", err)
		}
		if token == "" || principal.CompanyID != companyOne {
			t.Fatalf("unexpected login result: %#v", principal)
		}
		authenticated, err := authService.Authenticate(ctx, token)
		if err != nil || authenticated.CompanyID != companyOne {
			t.Fatalf("authenticate: principal=%#v err=%v", authenticated, err)
		}
		if err := authService.Logout(ctx, token); err != nil {
			t.Fatalf("logout: %v", err)
		}
		if _, err := authService.Authenticate(ctx, token); !errors.Is(err, auth.ErrInvalidSession) {
			t.Fatalf("revoked session accepted: %v", err)
		}
	})

	t.Run("wrong password and disabled user", func(t *testing.T) {
		if _, _, err := authService.Login(ctx, "admin-"+suffix+"@example.test", "wrong-password", companyOne); !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("wrong password result: %v", err)
		}
		if _, _, err := authService.Login(ctx, "disabled-"+suffix+"@example.test", password, companyOne); !errors.Is(err, auth.ErrInactiveAccess) {
			t.Fatalf("disabled user result: %v", err)
		}
	})

	admin := auth.Principal{UserID: adminID, CompanyID: companyOne}
	limited := auth.Principal{UserID: limitedID, CompanyID: companyOne}

	t.Run("permissions update without session refresh", func(t *testing.T) {
		if err := authorizer.RequirePermission(ctx, admin, "roles.manage"); err != nil {
			t.Fatalf("administrator denied: %v", err)
		}
		if err := authorizer.RequirePermission(ctx, limited, "employees.view"); !errors.Is(err, authorization.ErrPermissionDenied) {
			t.Fatalf("limited user unexpectedly granted: %v", err)
		}
		mustExec(t, db, `INSERT INTO role_permissions (company_id,role_id,permission_key) VALUES ($1,$2,'employees.view')`, companyOne, limitedRole)
		if err := authorizer.RequirePermission(ctx, limited, "employees.view"); err != nil {
			t.Fatalf("new permission not effective: %v", err)
		}
		mustExec(t, db, `DELETE FROM role_permissions WHERE company_id=$1 AND role_id=$2 AND permission_key='employees.view'`, companyOne, limitedRole)
		if err := authorizer.RequirePermission(ctx, limited, "employees.view"); !errors.Is(err, authorization.ErrPermissionDenied) {
			t.Fatalf("removed permission still effective: %v", err)
		}
	})

	t.Run("module entitlements are tenant scoped", func(t *testing.T) {
		if err := authorizer.RequireModule(ctx, admin, "core"); err != nil {
			t.Fatalf("core denied: %v", err)
		}
		if err := authorizer.RequireModule(ctx, admin, "inventory"); !errors.Is(err, authorization.ErrModuleUnavailable) {
			t.Fatalf("missing entitlement result: %v", err)
		}
		mustExec(t, db, `INSERT INTO module_entitlements (company_id,module_key) VALUES ($1,'inventory')`, companyOne)
		if err := authorizer.RequireModule(ctx, admin, "inventory"); err != nil {
			t.Fatalf("enabled entitlement denied: %v", err)
		}
		if err := authorizer.RequireModule(ctx, auth.Principal{UserID: adminID, CompanyID: companyTwo}, "inventory"); !errors.Is(err, authorization.ErrModuleUnavailable) {
			t.Fatalf("cross-company entitlement leaked: %v", err)
		}
	})

	t.Run("tenant queries and audit stay scoped", func(t *testing.T) {
		employees, err := service.ListEmployees(ctx, admin)
		if err != nil {
			t.Fatalf("list employees: %v", err)
		}
		if len(employees) != 1 || employees[0].DisplayName != "Tenant A Employee" {
			t.Fatalf("cross-company employee visible: %#v", employees)
		}
		created, err := service.CreateEmployee(ctx, admin, "Audited Employee", nil)
		if err != nil {
			t.Fatalf("create employee: %v", err)
		}
		var actor, company, action, target string
		if err := db.QueryRow(ctx, `SELECT actor_user_id,company_id,action,target_id FROM audit_logs WHERE company_id=$1 AND target_id=$2`, companyOne, created.ID).Scan(&actor, &company, &action, &target); err != nil {
			t.Fatalf("read audit log: %v", err)
		}
		if actor != adminID || company != companyOne || action != "employee.created" || target != created.ID {
			t.Fatalf("incorrect audit entry: %s %s %s %s", actor, company, action, target)
		}
	})

	t.Run("disabled company access invalidates session", func(t *testing.T) {
		token, _, err := authService.Login(ctx, "limited-"+suffix+"@example.test", password, companyOne)
		if err != nil {
			t.Fatalf("login before disable: %v", err)
		}
		if err := service.SetUserAccessStatus(ctx, admin, limitedID, "disabled"); err != nil {
			t.Fatalf("disable access: %v", err)
		}
		if _, err := authService.Authenticate(ctx, token); !errors.Is(err, auth.ErrInvalidSession) {
			t.Fatalf("disabled access session accepted: %v", err)
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

func cleanupFixture(t *testing.T, db *pgxpool.Pool, companyOne, companyTwo string, userIDs ...string) {
	t.Helper()
	ctx := context.Background()
	for _, table := range []string{"audit_logs", "sessions", "module_entitlements", "company_user_roles", "role_permissions", "employees", "roles", "company_users"} {
		if _, err := db.Exec(ctx, "DELETE FROM "+table+" WHERE company_id = ANY($1::uuid[])", []string{companyOne, companyTwo}); err != nil {
			t.Errorf("cleanup %s: %v", table, err)
		}
	}
	if _, err := db.Exec(ctx, `DELETE FROM users WHERE id = ANY($1::uuid[])`, userIDs); err != nil {
		t.Errorf("cleanup users: %v", err)
	}
	if _, err := db.Exec(ctx, `DELETE FROM companies WHERE id = ANY($1::uuid[])`, []string{companyOne, companyTwo}); err != nil {
		t.Errorf("cleanup companies: %v", err)
	}
}
