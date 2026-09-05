package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/printeragent"
)

func main() {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("COMMERCEOPS_URL")), "/")
	credential := strings.TrimSpace(os.Getenv("PRINTER_AGENT_CREDENTIAL"))
	journalPath := strings.TrimSpace(os.Getenv("PRINTER_AGENT_JOURNAL"))
	if journalPath == "" {
		journalPath = "./printer-agent-journal.jsonl"
	}
	if base == "" || credential == "" {
		log.Fatal("COMMERCEOPS_URL and PRINTER_AGENT_CREDENTIAL are required")
	}
	journal, err := printeragent.OpenJournal(journalPath)
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	client := &printeragent.Client{BaseURL: base, Credential: credential, HTTP: &http.Client{Timeout: 30 * time.Second}}
	runner := &printeragent.Runner{Client: client, Backend: printeragent.NewCUPS(), Journal: journal}
	if err = printeragent.Run(ctx, runner, 5*time.Second); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}
