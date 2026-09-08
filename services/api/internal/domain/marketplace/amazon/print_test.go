// This file verifies marketplace print composition and guards its document-level contract in the Amazon marketplace adapter.
package amazon

import (
	"context"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
	"github.com/commerceops/commerceops/services/api/internal/platform/pdfgenerator"
)

func TestAmazonPrintGeneratorEnrichesWithoutCroppingSource(t *testing.T) {
	pdf, err := os.ReadFile("testdata/sanitized_label_invoice.pdf")
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewPrintGenerator().Generate(context.Background(), []pdfgenerator.Page{{SourceID: "sanitized", PDF: pdf, Number: 1, InvoiceNumber: 2, SKU: "SANITIZED-AMZ-1", Quantity: 2}}, true)
	if err != nil {
		t.Fatal(err)
	}
	labels, err := pdfextractor.NewPoppler().Extract(context.Background(), result.Labels)
	if err != nil || len(labels) != 1 || !strings.Contains(labels[0].Text, "SKU: SANITIZED-AMZ-1 | QTY: 2") {
		t.Fatalf("label enrichment missing: pages=%d err=%v", len(labels), err)
	}
	invoices, err := pdfextractor.NewPoppler().Extract(context.Background(), result.Invoices)
	if err != nil || len(invoices) != 1 || !strings.Contains(invoices[0].Text, "Tax Invoice") {
		t.Fatalf("invoice output invalid: pages=%d err=%v", len(invoices), err)
	}
	assertA4AndSourceInk(t, result.Labels)
}

func TestAmazonPrintGeneratorRejectsUnvalidatedGeometryAndMissingValues(t *testing.T) {
	generator := NewPrintGenerator()
	if _, err := generator.Generate(context.Background(), []pdfgenerator.Page{{SourceID: "bad", PDF: []byte("%PDF-bad"), Number: 1, SKU: "SAFE", Quantity: 1}}, false); err == nil {
		t.Fatal("invalid geometry accepted")
	}
	pdf, err := os.ReadFile("testdata/sanitized_label_invoice.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = generator.Generate(context.Background(), []pdfgenerator.Page{{SourceID: "missing", PDF: pdf, Number: 1, SKU: "SAFE"}}, false); err == nil {
		t.Fatal("missing quantity accepted")
	}
}

func TestPrivateAmazonPrintableOutput(t *testing.T) {
	path := os.Getenv("AMAZON_PRIVATE_SAMPLE")
	if path == "" {
		t.Skip("AMAZON_PRIVATE_SAMPLE is not set")
	}
	pdf, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pages, err := pdfextractor.NewPopplerWithOCR().Extract(context.Background(), pdf)
	if err != nil {
		t.Fatal(err)
	}
	documents, err := Parse(pages)
	if err != nil || len(documents) != 5 {
		t.Fatalf("normalized documents=%d err=%v", len(documents), err)
	}
	inputs := make([]pdfgenerator.Page, 0, len(documents))
	for _, document := range documents {
		invoice := 0
		for _, source := range document.Sources {
			if source.Role == "invoice" {
				invoice = source.Page
			}
		}
		if document.SKU == "" || document.Quantity == nil || invoice == 0 {
			t.Fatal("private association is not printable")
		}
		inputs = append(inputs, pdfgenerator.Page{SourceID: "private", PDF: pdf, Number: document.Page, InvoiceNumber: invoice, SKU: document.SKU, Quantity: *document.Quantity})
	}
	result, err := NewPrintGenerator().Generate(context.Background(), inputs, true)
	if err != nil {
		t.Fatal(err)
	}
	labels, err := pdfextractor.NewPoppler().Extract(context.Background(), result.Labels)
	if err != nil || len(labels) != 5 {
		t.Fatalf("printable labels=%d err=%v", len(labels), err)
	}
	for _, page := range labels {
		if !strings.Contains(page.Text, "SKU:") || !strings.Contains(page.Text, "QTY:") {
			t.Fatal("private printable enrichment missing")
		}
	}
	assertA4AndSourceInk(t, result.Labels)
	assertRenderedShippingIdentifiers(t, result.Labels)
	invoices, err := pdfextractor.NewPoppler().Extract(context.Background(), result.Invoices)
	if err != nil || len(invoices) != 5 {
		t.Fatalf("printable invoices=%d err=%v", len(invoices), err)
	}
}

func assertRenderedShippingIdentifiers(t *testing.T, pdf []byte) {
	t.Helper()
	dir := t.TempDir()
	name := filepath.Join(dir, "labels.pdf")
	if err := os.WriteFile(name, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	prefix := filepath.Join(dir, "page")
	if output, err := exec.Command("pdftoppm", "-f", "1", "-l", "1", "-singlefile", "-r", "200", "-png", name, prefix).CombinedOutput(); err != nil {
		t.Fatalf("render private output: %s err=%v", output, err)
	}
	text, err := exec.Command("tesseract", prefix+".png", "stdout", "--psm", "6").CombinedOutput()
	if err != nil || uniqueOrderID(string(text)) == "" || uniqueCapture(string(text), awbRE, ocrAWBRE) == "" {
		t.Fatal("rendered output did not preserve recognizable shipping identifiers")
	}
}

func assertA4AndSourceInk(t *testing.T, pdf []byte) {
	t.Helper()
	dir := t.TempDir()
	name := filepath.Join(dir, "labels.pdf")
	if err := os.WriteFile(name, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := exec.Command("pdfinfo", name).CombinedOutput()
	if err != nil || !strings.Contains(string(info), "595 x 842 pts") {
		t.Fatalf("unexpected geometry: %s err=%v", info, err)
	}
	prefix := filepath.Join(dir, "render")
	if output, err := exec.Command("pdftoppm", "-f", "1", "-l", "1", "-singlefile", "-r", "72", "-png", name, prefix).CombinedOutput(); err != nil {
		t.Fatalf("render output: %s err=%v", output, err)
	}
	file, err := os.Open(prefix + ".png")
	if err != nil {
		t.Fatal(err)
	}
	image, err := png.Decode(file)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	dark := 0
	bounds := image.Bounds()
	for y := bounds.Min.Y + 90; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := image.At(x, y).RGBA()
			if r+g+b < 3*0x5000 {
				dark++
			}
		}
	}
	if dark < 300 {
		t.Fatalf("source label content was not preserved: dark_pixels=%d", dark)
	}
}
