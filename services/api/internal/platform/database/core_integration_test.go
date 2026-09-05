// This file contains PostgreSQL-backed tests for cross-layer behavior, tenant isolation, and domain invariants in the PostgreSQL infrastructure layer.
package database_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestCoreSchemaRejectsCrossTenantAssociations(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	defer conn.Close(ctx)

	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck -- the test transaction is always disposable

	var companyOneID, companyTwoID, userID, roleID string
	mustQueryRow(t, tx, "INSERT INTO companies (name) VALUES ('Tenant One') RETURNING id", &companyOneID)
	mustQueryRow(t, tx, "INSERT INTO companies (name) VALUES ('Tenant Two') RETURNING id", &companyTwoID)
	mustQueryRow(t, tx, "INSERT INTO users (email, password_hash) VALUES ('tenant-test@example.com', 'test-password-hash') RETURNING id", &userID)

	mustExec(t, tx, "INSERT INTO company_users (company_id, user_id) VALUES ($1, $2)", companyOneID, userID)
	mustQueryRow(t, tx, "INSERT INTO roles (company_id, name) VALUES ($1, 'Manager') RETURNING id", &roleID, companyOneID)

	expectForeignKeyViolation(t, ctx, tx,
		"INSERT INTO employees (company_id, user_id, display_name) VALUES ($1, $2, 'Cross Tenant Employee')",
		companyTwoID, userID,
	)
	expectForeignKeyViolation(t, ctx, tx,
		"INSERT INTO role_permissions (company_id, role_id, permission_key) VALUES ($1, $2, 'employees.view')",
		companyTwoID, roleID,
	)
	expectForeignKeyViolation(t, ctx, tx,
		"INSERT INTO company_user_roles (company_id, user_id, role_id) VALUES ($1, $2, $3)",
		companyTwoID, userID, roleID,
	)
	expectForeignKeyViolation(t, ctx, tx,
		"INSERT INTO audit_logs (company_id, actor_user_id, action, target_type, target_id) VALUES ($1, $2, 'test', 'test', 'test')",
		companyTwoID, userID,
	)
}

func expectForeignKeyViolation(t *testing.T, ctx context.Context, tx pgx.Tx, query string, args ...any) {
	t.Helper()

	mustExec(t, tx, "SAVEPOINT expected_constraint_failure")
	_, err := tx.Exec(ctx, query, args...)
	if err == nil {
		t.Fatal("cross-tenant write unexpectedly succeeded")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Fatalf("expected foreign key violation (23503), got %v", err)
	}

	mustExec(t, tx, "ROLLBACK TO SAVEPOINT expected_constraint_failure")
	mustExec(t, tx, "RELEASE SAVEPOINT expected_constraint_failure")
}

func mustExec(t *testing.T, tx pgx.Tx, query string, args ...any) {
	t.Helper()
	if _, err := tx.Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("execute test fixture query: %v", err)
	}
}

func mustQueryRow(t *testing.T, tx pgx.Tx, query string, destination *string, args ...any) {
	t.Helper()
	if err := tx.QueryRow(context.Background(), query, args...).Scan(destination); err != nil {
		t.Fatalf("execute test fixture query: %v", err)
	}
}
