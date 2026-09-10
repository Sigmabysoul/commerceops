// This file verifies local-only configuration guards used by the one-click launcher.
package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func TestEnsureEnvironmentCreatesPrivateLocalConfiguration(t *testing.T) {
	root := t.TempDir()
	if err := ensureEnvironment(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".env")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	values, err := readEnvironment(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = validateLocalConfiguration(values); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(values["COMMERCEOPS_POSTGRES_PASSWORD"], "replace-with") || len(values["COMMERCEOPS_POSTGRES_PASSWORD"]) < 20 {
		t.Fatal("generated database password is missing or unsafe")
	}
}

func TestEnsureEnvironmentNeverOverwritesExistingConfiguration(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".env")
	if err := os.WriteFile(path, []byte("sentinel=true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureEnvironment(root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "sentinel=true\n" {
		t.Fatalf("existing .env changed: %q", data)
	}
}

func TestReadEnvironmentAndLocalDatabaseGuard(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	data := "# local\nAPP_ENV=development\nDATABASE_URL=\"postgres://user:pass@127.0.0.1:5432/app?sslmode=disable\"\nCOMMERCEOPS_POSTGRES_PASSWORD='safe-local'\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	values, err := readEnvironment(path)
	if err != nil {
		t.Fatal(err)
	}
	values["COMMERCEOPS_POSTGRES_USER"] = "user"
	values["COMMERCEOPS_POSTGRES_DB"] = "app"
	values["COMMERCEOPS_POSTGRES_PASSWORD"] = "pass"
	values["HTTP_ADDR"] = ":8080"
	values["NEXT_PUBLIC_API_BASE_URL"] = "http://localhost:8080"
	values["OBJECT_STORAGE_DRIVER"] = "local"
	if values["DATABASE_URL"] != "postgres://user:pass@127.0.0.1:5432/app?sslmode=disable" {
		t.Fatalf("DATABASE_URL=%q", values["DATABASE_URL"])
	}
	if err = validateLocalConfiguration(values); err != nil {
		t.Fatal(err)
	}

	values["DATABASE_URL"] = "postgres://user:pass@database.example.com/app"
	if err = validateLocalConfiguration(values); err == nil || !strings.Contains(err.Error(), "non-local") {
		t.Fatalf("remote database error=%v", err)
	}
	values["DATABASE_URL"] = "postgres://user:pass@localhost/app"
	values["APP_ENV"] = "production"
	if err = validateLocalConfiguration(values); err == nil || !strings.Contains(err.Error(), "development") {
		t.Fatalf("production environment error=%v", err)
	}
}

func TestBootstrapPostgreSQL(t *testing.T) {
	connection := os.Getenv("TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, connection)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	schema := fmt.Sprintf("launcher_test_%d", time.Now().UnixNano())
	if _, err = pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, e := pool.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); e != nil {
			t.Error(e)
		}
	}()
	parsed, err := url.Parse(connection)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	q.Set("search_path", schema)
	parsed.RawQuery = q.Encode()
	isolated, err := pgxpool.New(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	defer isolated.Close()
	migrations, err := filepath.Glob("../../migrations/*.up.sql")
	if err != nil || len(migrations) == 0 {
		t.Fatal("migration files missing", err)
	}
	for _, path := range migrations {
		data, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = isolated.Exec(ctx, string(data)); e != nil {
			t.Fatal(path, e)
		}
	}
	credentialsPath := filepath.Join(t.TempDir(), "credentials.txt")
	credentials, created, err := bootstrapLocalAdministrator(parsed.String(), credentialsPath)
	if err != nil || !created {
		t.Fatal(created, err)
	}
	data, err := os.ReadFile(credentialsPath)
	if err != nil || string(data) != credentials {
		t.Fatal("credentials not persisted", err)
	}
	password := strings.TrimPrefix(strings.Split(strings.TrimSpace(credentials), "\n")[1], "Password: ")
	var hash string
	if err = isolated.QueryRow(ctx, "SELECT password_hash FROM users").Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		t.Fatal(err)
	}
	again, created, err := bootstrapLocalAdministrator(parsed.String(), credentialsPath)
	if err != nil || created || again != "" {
		t.Fatal("bootstrap must not overwrite existing users", created, err)
	}
	var users int
	if err = isolated.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&users); err != nil || users != 1 {
		t.Fatal(users, err)
	}
}
func TestProcessStartRecognizesCurrentProcess(t *testing.T) {
	if processStart(os.Getpid()) == "" {
		t.Fatal("missing current process identity")
	}
	if processStart(-1) != "" {
		t.Fatal("invalid PID has identity")
	}
}

func TestStaleProcessRecordDoesNotSignalReusedPID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "processes.json")
	if err := writeProcessState(path, processState{ServerPID: os.Getpid(), ServerStart: "not-this-process"}); err != nil {
		t.Fatal(err)
	}
	if err := stopRecordedProcesses(path, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("stale record not cleared", err)
	}
}
func TestDatabaseQueryCannotOverrideLocalHost(t *testing.T) {
	root := t.TempDir()
	if err := ensureEnvironment(root); err != nil {
		t.Fatal(err)
	}
	values, err := readEnvironment(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	values["DATABASE_URL"] += "&host=remote.example"
	if err = validateLocalConfiguration(values); err == nil {
		t.Fatal("query host override accepted")
	}
}
