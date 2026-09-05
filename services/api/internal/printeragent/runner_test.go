// This file tests printer-agent polling, retry, cancellation, and acknowledgement behavior in the local printer-agent package.
package printeragent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
)

type recordingBackend struct {
	mu          sync.Mutex
	submissions int
}

func (b *recordingBackend) List(context.Context) ([]DiscoveredPrinter, error) {
	return []DiscoveredPrinter{{OSPrinterID: "Safe_1", SuggestedName: "Safe 1"}}, nil
}
func (b *recordingBackend) SubmitPDF(_ context.Context, printer string, copies int, filename string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if printer != "Safe_1" || copies != 2 || filename == "" {
		return io.ErrUnexpectedEOF
	}
	b.submissions++
	return nil
}

func TestRunnerReconnectNeverResubmitsJournaledJob(t *testing.T) {
	artifact := []byte("%PDF-safe-agent")
	sum := sha256.Sum256(artifact)
	var mu sync.Mutex
	reports := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer safe-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/v1/printer-agent/heartbeat":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"printers":[]}`))
		case "/api/v1/printer-agent/jobs/claim":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"claim": map[string]any{"job": map[string]any{"id": "11111111-1111-4111-8111-111111111111", "copies": 2}, "lease_token": "lease", "artifact_sha256": hex.EncodeToString(sum[:]), "artifact_size": len(artifact), "os_printer_id": "Safe_1"}})
		case "/api/v1/printer-agent/jobs/11111111-1111-4111-8111-111111111111/artifact":
			if r.Header.Get("X-Print-Lease") != "lease" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write(artifact)
		case "/api/v1/printer-agent/jobs/11111111-1111-4111-8111-111111111111/status":
			var body struct {
				Status string `json:"status"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			reports = append(reports, body.Status)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"printer_job":{}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	journal, err := OpenJournal(filepath.Join(t.TempDir(), "journal.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	backend := &recordingBackend{}
	runner := &Runner{Client: &Client{BaseURL: server.URL, Credential: "safe-token", HTTP: server.Client()}, Backend: backend, Journal: journal}
	if err = runner.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = runner.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if backend.submissions != 1 {
		t.Fatalf("submissions=%d", backend.submissions)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(reports) != 3 || reports[0] != "printing" || reports[1] != "completed" || reports[2] != "completed" {
		t.Fatalf("reports=%v", reports)
	}
}
