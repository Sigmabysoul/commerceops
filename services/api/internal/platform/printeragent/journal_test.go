// This file tests durable idempotency records and recovery from malformed journal data in the local printer-agent package.
package printeragent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestJournalPersistsSubmissionBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.jsonl")
	j, err := OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = j.Record("job-1", "submitted"); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.State("job-1"); got != "submitted" {
		t.Fatalf("state=%q", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions=%o", info.Mode().Perm())
	}
}
func TestCUPSRejectsUntrustedArguments(t *testing.T) {
	cups := NewCUPS()
	for _, printer := range []string{"", "-evil", "../../printer", "printer name", "x;touch"} {
		if err := cups.SubmitPDF(context.Background(), printer, 1, "safe.pdf"); err == nil {
			t.Fatalf("accepted printer %q", printer)
		}
	}
	if err := cups.SubmitPDF(context.Background(), "Safe_Printer", 101, "safe.pdf"); err == nil {
		t.Fatal("accepted excessive copies")
	}
}
