// This file contains focused regression tests for the behavior owned by this package in the printing package.
package printing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/platform/objectstorage"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	db                                  *pgxpool.Pool
	service                             *Service
	company, other, user, role, product string
	p, otherP                           auth.Principal
	credential                          string
	agent                               Agent
	printer                             Printer
	pdf                                 []byte
	storage                             *objectstorage.Local
}

func setup(t *testing.T) *fixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprint(time.Now().UnixNano())
	f := &fixture{db: db}
	scan(t, db, `INSERT INTO companies(name) VALUES($1) RETURNING id`, "Printing "+suffix, &f.company)
	scan(t, db, `INSERT INTO companies(name) VALUES($1) RETURNING id`, "Printing other "+suffix, &f.other)
	scan(t, db, `INSERT INTO users(email,password_hash) VALUES($1,'test') RETURNING id`, "printing-"+suffix+"@example.test", &f.user)
	execSQL(t, db, `INSERT INTO company_users(company_id,user_id) VALUES($1,$3),($2,$3)`, f.company, f.other, f.user)
	scan(t, db, `INSERT INTO roles(company_id,name) VALUES($1,'Printing operator') RETURNING id`, f.company, &f.role)
	execSQL(t, db, `INSERT INTO role_permissions(company_id,role_id,permission_key) SELECT $1,$2,key FROM permissions WHERE key IN ('printers.view','printers.manage','printing.print','printing.reprint','print_library.view','print_library.manage')`, f.company, f.role)
	execSQL(t, db, `INSERT INTO company_user_roles(company_id,user_id,role_id) VALUES($1,$2,$3)`, f.company, f.user, f.role)
	scan(t, db, `INSERT INTO products(company_id,internal_code,name) VALUES($1,'PRINT-R1','Print R1') RETURNING id`, f.company, &f.product)
	storage, err := objectstorage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f.service = NewService(db, authorization.NewService(db), storage)
	f.storage = storage
	f.p = auth.Principal{CompanyID: f.company, UserID: f.user}
	f.otherP = auth.Principal{CompanyID: f.other, UserID: f.user}
	var otherRole string
	scan(t, db, `INSERT INTO roles(company_id,name) VALUES($1,'Other printing operator') RETURNING id`, f.other, &otherRole)
	execSQL(t, db, `INSERT INTO role_permissions(company_id,role_id,permission_key) SELECT $1,$2,key FROM permissions WHERE key IN ('printers.view','printing.print','print_library.view')`, f.other, otherRole)
	execSQL(t, db, `INSERT INTO company_user_roles(company_id,user_id,role_id) VALUES($1,$2,$3)`, f.other, f.user, otherRole)
	created, err := f.service.CreateAgent(ctx, f.p, "Warehouse Linux", "linux_cups")
	if err != nil {
		t.Fatal(err)
	}
	f.credential = created.Credential
	f.agent = created.Agent
	printers, err := f.service.Heartbeat(ctx, AgentPrincipal{CompanyID: f.company, AgentID: f.agent.ID}, []LocalPrinter{{OSPrinterID: "Zebra_1", SuggestedName: "Packing Zebra", Capabilities: map[string]any{"media": "4x6"}}})
	if err != nil || len(printers) != 1 {
		t.Fatalf("printers=%v err=%v", printers, err)
	}
	f.printer = printers[0]
	f.pdf, err = os.ReadFile(filepath.Join("..", "marketplace", "amazon", "testdata", "sanitized_label_invoice.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return f
}

func TestPrintingPlatformLifecycleIdempotencyAndNeutrality(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	authenticated, err := f.service.AuthenticateAgent(ctx, f.credential)
	if err != nil || authenticated.AgentID != f.agent.ID {
		t.Fatalf("agent=%v err=%v", authenticated, err)
	}
	var rawHash []byte
	scan(t, f.db, `SELECT token_hash FROM printer_agent_credentials WHERE agent_id=$1`, f.agent.ID, &rawHash)
	if string(rawHash) == f.credential {
		t.Fatal("credential stored in plaintext")
	}
	asset, err := f.service.CreateAsset(ctx, f.p, "Fragile", "Handling", "Safe reusable label", &f.printer.ID, 2, &f.product, true, "fragile.pdf", f.pdf)
	if err != nil {
		t.Fatal(err)
	}
	description := "Updated reusable label"
	updatedAsset, err := f.service.UpdateAsset(ctx, f.p, asset.ID, UpdateAssetInput{Name: "Fragile", Category: "Handling", Description: &description, DefaultPrinterID: &f.printer.ID, DefaultCopies: 2, ProductID: &f.product, Favorite: true, Active: true})
	if err != nil || updatedAsset.SHA256 != asset.SHA256 || updatedAsset.Description == nil || *updatedAsset.Description != description {
		t.Fatalf("updated asset=%v err=%v", updatedAsset, err)
	}
	assets, err := f.service.ListAssets(ctx, f.p, "Frag", "Handling")
	if err != nil || len(assets) != 1 || assets[0].ID != asset.ID {
		t.Fatalf("assets=%v err=%v", assets, err)
	}
	if otherAssets, listErr := f.service.ListAssets(ctx, f.otherP, "", ""); listErr != nil || len(otherAssets) != 0 {
		t.Fatalf("cross tenant assets=%v err=%v", otherAssets, listErr)
	}
	if _, _, crossErr := f.service.CreateQuickJob(ctx, f.otherP, CreateJobInput{PrinterID: f.printer.ID, AssetID: asset.ID, Copies: 1, IdempotencyKey: "cross-tenant"}); !errors.Is(crossErr, ErrNotFound) {
		t.Fatalf("cross tenant job=%v", crossErr)
	}
	input := CreateJobInput{PrinterID: f.printer.ID, AssetID: asset.ID, Copies: 2, IdempotencyKey: "quick-" + asset.ID}
	job, replay, err := f.service.CreateQuickJob(ctx, f.p, input)
	if err != nil || replay {
		t.Fatalf("job=%v replay=%v err=%v", job, replay, err)
	}
	same, replay, err := f.service.CreateQuickJob(ctx, f.p, input)
	if err != nil || !replay || same.ID != job.ID {
		t.Fatalf("same=%v replay=%v err=%v", same, replay, err)
	}
	input.Copies = 3
	if _, _, err = f.service.CreateQuickJob(ctx, f.p, input); !errors.Is(err, ErrConflict) {
		t.Fatalf("idempotency conflict=%v", err)
	}
	if _, _, err = f.service.CreateQuickJob(ctx, f.p, CreateJobInput{PrinterID: f.printer.ID, AssetID: asset.ID, Copies: 21, IdempotencyKey: "large"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("large copies=%v", err)
	}
	claim, err := f.service.Claim(ctx, authenticated)
	if err != nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%v err=%v", claim, err)
	}
	data, err := f.service.AgentDownload(ctx, authenticated, job.ID, claim.LeaseToken)
	if err != nil || len(data) != len(f.pdf) {
		t.Fatalf("download=%d err=%v", len(data), err)
	}
	if _, err = f.service.Report(ctx, authenticated, job.ID, claim.LeaseToken, "printing", "", ""); err != nil {
		t.Fatal(err)
	}
	done, err := f.service.Report(ctx, authenticated, job.ID, claim.LeaseToken, "completed", "", "")
	if err != nil || done.Status != "completed" {
		t.Fatalf("done=%v err=%v", done, err)
	}
	if _, err = f.service.Report(ctx, authenticated, job.ID, claim.LeaseToken, "completed", "", ""); err != nil {
		t.Fatalf("completion replay=%v", err)
	}
	var inventory, audits, events int
	scan(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, f.company, &inventory)
	scan(t, f.db, `SELECT count(*) FROM audit_logs WHERE company_id=$1 AND target_type IN ('printer_agent','print_library_asset','printer_job')`, f.company, &audits)
	scan(t, f.db, `SELECT count(*) FROM printer_job_events WHERE company_id=$1 AND printer_job_id=$2`, f.company, job.ID, &events)
	if inventory != 0 || audits < 4 || events != 4 {
		t.Fatalf("inventory=%d audits=%d events=%d", inventory, audits, events)
	}
}

func TestFailedJobExplicitRetryAndConcurrentClaim(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	asset, err := f.service.CreateAsset(ctx, f.p, "Carton", "Carton", "", &f.printer.ID, 1, nil, false, "carton.pdf", f.pdf)
	if err != nil {
		t.Fatal(err)
	}
	job, _, err := f.service.CreateQuickJob(ctx, f.p, CreateJobInput{PrinterID: f.printer.ID, AssetID: asset.ID, Copies: 1, IdempotencyKey: "fail-" + asset.ID})
	if err != nil {
		t.Fatal(err)
	}
	agent := AgentPrincipal{CompanyID: f.company, AgentID: f.agent.ID}
	var wg sync.WaitGroup
	claims := make(chan Claim, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c, e := f.service.Claim(ctx, agent); e == nil {
				claims <- c
			}
		}()
	}
	wg.Wait()
	close(claims)
	var got []Claim
	for c := range claims {
		got = append(got, c)
	}
	if len(got) != 1 || got[0].Job.ID != job.ID {
		t.Fatalf("claims=%v", got)
	}
	if _, err = f.service.Report(ctx, agent, job.ID, got[0].LeaseToken, "failed", "paper_out", "Printer is out of paper"); err != nil {
		t.Fatal(err)
	}
	retry, replayed, err := f.service.Retry(ctx, f.p, job.ID, "retry-"+job.ID)
	if err != nil || replayed || retry.SourceJobID == nil || *retry.SourceJobID != job.ID {
		t.Fatalf("retry=%v replay=%v err=%v", retry, replayed, err)
	}
	retryClaim, err := f.service.Claim(ctx, agent)
	if err != nil || retryClaim.Job.ID != retry.ID {
		t.Fatalf("retry claim=%v err=%v", retryClaim, err)
	}
	execSQL(t, f.db, `UPDATE printer_jobs SET lease_expires_at=now()-interval '1 second' WHERE company_id=$1 AND id=$2`, f.company, retry.ID)
	if _, err = f.service.Claim(ctx, agent); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired claim should not auto-requeue: %v", err)
	}
	expired, err := f.service.getJob(ctx, f.company, retry.ID)
	if err != nil || expired.Status != "failed" || expired.FailureCode == nil || *expired.FailureCode != "lease_expired" {
		t.Fatalf("expired=%v err=%v", expired, err)
	}
	var inventory int
	scan(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, f.company, &inventory)
	if inventory != 0 {
		t.Fatalf("retry changed inventory: %d", inventory)
	}
}

func TestExistingBatchArtifactUsesCanonicalQueue(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var batchID, generationJobID, artifactID string
	scan(t, f.db, `INSERT INTO batches(company_id,marketplace_key,created_by,status,idempotency_key,request_hash,ready_at) VALUES($1,'flipkart',$2,'ready',$3,repeat('b',64),now()) RETURNING id`, f.company, f.user, "batch-artifact-"+fmt.Sprint(time.Now().UnixNano()), &batchID)
	scan(t, f.db, `INSERT INTO print_jobs(company_id,batch_id,requested_by,status,sort_labels,export_invoices,generation_version,idempotency_key,request_hash,completed_at) VALUES($1,$2,$3,'ready',false,false,'test-v1',$4,repeat('a',64),now()) RETURNING id`, f.company, batchID, f.user, "generation-"+batchID, &generationJobID)
	key := f.company + "/print-jobs/" + generationJobID + "/labels.pdf"
	if err := f.storage.Put(ctx, key, bytes.NewReader(f.pdf), int64(len(f.pdf)), "application/pdf"); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(f.pdf)
	scan(t, f.db, `INSERT INTO print_artifacts(company_id,print_job_id,kind,storage_key,size_bytes,sha256,page_count) VALUES($1,$2,'labels',$3,$4,$5,1) RETURNING id`, f.company, generationJobID, key, len(f.pdf), hex.EncodeToString(sum[:]), &artifactID)
	job, replay, err := f.service.QueueArtifact(ctx, f.p, QueueArtifactInput{PrinterID: f.printer.ID, ArtifactID: artifactID, Copies: 1, IdempotencyKey: "artifact-" + artifactID})
	if err != nil || replay || job.ArtifactID == nil || *job.ArtifactID != artifactID || job.OriginType != "ecommerce_batch" {
		t.Fatalf("job=%v replay=%v err=%v", job, replay, err)
	}
	var inventory int
	scan(t, f.db, `SELECT count(*) FROM inventory_transactions WHERE company_id=$1`, f.company, &inventory)
	if inventory != 0 {
		t.Fatalf("artifact queue changed inventory: %d", inventory)
	}
}

func TestPrinterPresenceManagementAndCredentialRevocation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	location := "Packing desk 2"
	updated, err := f.service.UpdatePrinter(ctx, f.p, f.printer.ID, UpdatePrinterInput{FriendlyName: "Dispatch Zebra", Location: &location, Enabled: false})
	if err != nil || updated.Enabled || updated.FriendlyName != "Dispatch Zebra" {
		t.Fatalf("updated=%v err=%v", updated, err)
	}
	if _, err = f.service.UpdatePrinter(ctx, f.p, f.printer.ID, UpdatePrinterInput{FriendlyName: "Dispatch Zebra", Location: &location, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	execSQL(t, f.db, `UPDATE registered_printers SET last_seen_at=now()-interval '5 minutes' WHERE company_id=$1 AND id=$2`, f.company, f.printer.ID)
	printers, err := f.service.ListPrinters(ctx, f.p)
	if err != nil || len(printers) != 1 || printers[0].Status != "offline" {
		t.Fatalf("printers=%v err=%v", printers, err)
	}
	if err = f.service.RevokeAgent(ctx, f.p, f.agent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.AuthenticateAgent(ctx, f.credential); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("revoked credential=%v", err)
	}
}

func TestInvalidPDFAndPermissionBoundaries(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, err := f.service.CreateAsset(ctx, f.p, "Bad", "Bad", "", nil, 1, nil, false, "bad.pdf", []byte("%PDF-bad")); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid PDF=%v", err)
	}
	execSQL(t, f.db, `DELETE FROM role_permissions WHERE company_id=$1 AND role_id=$2 AND permission_key='printing.print'`, f.company, f.role)
	if _, _, err := f.service.CreateQuickJob(ctx, f.p, CreateJobInput{}); !errors.Is(err, authorization.ErrPermissionDenied) {
		t.Fatalf("permission=%v", err)
	}
}

func TestPrintingMigrationUpDown(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	root := filepath.Join("..", "..", "migrations")
	schema := "p13_migration_" + fmt.Sprint(time.Now().UnixNano())
	if _, err = tx.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SET LOCAL search_path TO `+schema+`,public`); err != nil {
		t.Fatal(err)
	}
	for number := 1; number <= 21; number++ {
		matches, globErr := filepath.Glob(filepath.Join(root, fmt.Sprintf("%06d_*.up.sql", number)))
		if globErr != nil || len(matches) != 1 {
			t.Fatalf("migration %d matches=%v err=%v", number, matches, globErr)
		}
		data, readErr := os.ReadFile(matches[0])
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = tx.Exec(ctx, string(data)); err != nil {
			t.Fatalf("apply %s: %v", matches[0], err)
		}
	}
	var exists *string
	if err = tx.QueryRow(ctx, `SELECT to_regclass($1)`, schema+".printer_jobs").Scan(&exists); err != nil || exists == nil {
		t.Fatalf("up=%v err=%v", exists, err)
	}
	down, err := os.ReadFile(filepath.Join(root, "000021_printing_platform.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	exists = nil
	if err = tx.QueryRow(ctx, `SELECT to_regclass($1)`, schema+".printer_jobs").Scan(&exists); err != nil || exists != nil {
		t.Fatalf("down=%v err=%v", exists, err)
	}
}

func scan(t *testing.T, db *pgxpool.Pool, q string, args ...any) {
	t.Helper()
	targets := args[len(args)-1:]
	params := args[:len(args)-1]
	if err := db.QueryRow(context.Background(), q, params...).Scan(targets...); err != nil {
		t.Fatal(err)
	}
}
func execSQL(t *testing.T, db *pgxpool.Pool, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(context.Background(), q, args...); err != nil {
		t.Fatal(err)
	}
}
