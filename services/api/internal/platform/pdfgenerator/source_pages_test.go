package pdfgenerator_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
	"github.com/commerceops/commerceops/services/api/internal/platform/pdfgenerator"
)

func TestSourcePagesPreservesCompletePagesAndOrder(t *testing.T) {
	pdf, err := os.ReadFile("testdata/flipkart_a4.pdf")
	if err != nil {
		t.Fatal(err)
	}
	result, err := pdfgenerator.NewSourcePages().Generate(context.Background(), []pdfgenerator.Page{
		{SourceID: "fixture", PDF: pdf, Number: 2},
		{SourceID: "fixture", PDF: pdf, Number: 1},
	}, false)
	if err != nil || len(result.Labels) == 0 || result.Invoices != nil {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	pages, err := pdfextractor.NewPoppler().Extract(context.Background(), result.Labels)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 || !strings.Contains(pages[0].Text, "000000000012") || !strings.Contains(pages[0].Text, "Invoice No") || !strings.Contains(pages[1].Text, "000000000011") || !strings.Contains(pages[1].Text, "Invoice No") {
		t.Fatalf("complete source pages were not preserved in requested order")
	}
}

func TestSourcePagesRejectsInvoiceExportAndInvalidSources(t *testing.T) {
	pdf, err := os.ReadFile("testdata/flipkart_a4.pdf")
	if err != nil {
		t.Fatal(err)
	}
	generator := pdfgenerator.NewSourcePages()
	if _, err = generator.Generate(context.Background(), []pdfgenerator.Page{{SourceID: "fixture", PDF: pdf, Number: 1}}, true); err == nil {
		t.Fatal("invoice export was accepted without associated invoice evidence")
	}
	if _, err = generator.Generate(context.Background(), []pdfgenerator.Page{{SourceID: "bad", PDF: []byte("%PDF-bad"), Number: 1}}, false); err == nil {
		t.Fatal("invalid source PDF was accepted")
	}
}
