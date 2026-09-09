// Package testfixture supplies explicit sanitized seller accounts to legacy
// integration fixtures. It is imported only by tests, never production code.
package testfixture

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
)

func SeedAccounts(t *testing.T, db *pgxpool.Pool, company string) {
	t.Helper()
	ctx := context.Background()
	var identity string
	err := db.QueryRow(ctx, `INSERT INTO business_identities(company_id,display_name,status) VALUES($1,'Fixture seller identity','active') RETURNING id`, company).Scan(&identity)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO marketplace_accounts(company_id,marketplace_key,business_identity_id,internal_key,display_name,integration_mode,status) SELECT $1,key,$2,'fixture_'||key,'Fixture / '||display_name,'manual_upload','active' FROM marketplaces ON CONFLICT(company_id,internal_key) DO NOTHING`, company, identity)
	if err != nil {
		t.Fatal(err)
	}
}
func Account(t *testing.T, db *pgxpool.Pool, company, marketplace string) string {
	t.Helper()
	var id string
	if err := db.QueryRow(context.Background(), `SELECT id FROM marketplace_accounts WHERE company_id=$1 AND marketplace_key=$2 AND internal_key='fixture_'||$2`, company, marketplace).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
