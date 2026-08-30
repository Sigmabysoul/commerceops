package pdfgenerator_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
	"github.com/commerceops/commerceops/services/api/internal/platform/pdfgenerator"
)

func TestPopplerGeneratesNormalizedLabelsAndInvoices(t *testing.T) {
	pdf, err := os.ReadFile("testdata/flipkart_a4.pdf")
	if err != nil {
		t.Fatal(err)
	}
	result, err := pdfgenerator.NewPoppler().Generate(context.Background(), []pdfgenerator.Page{{SourceID: "fixture", PDF: pdf, Number: 2}, {SourceID: "fixture", PDF: pdf, Number: 1}}, true)
	if err != nil {
		t.Fatal(err)
	}
	labels, err := pdfextractor.NewPoppler().Extract(context.Background(), result.Labels)
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != 2 || !strings.Contains(labels[0].Text, "000000000012") || !strings.Contains(labels[1].Text, "000000000011") || strings.Contains(strings.Join([]string{labels[0].Text, labels[1].Text}, ""), "Invoice No") {
		t.Fatalf("unexpected label output")
	}
	invoices, err := pdfextractor.NewPoppler().Extract(context.Background(), result.Invoices)
	if err != nil {
		t.Fatal(err)
	}
	if len(invoices) != 2 || !strings.Contains(invoices[0].Text, "12") || !strings.Contains(invoices[1].Text, "11") {
		t.Fatalf("unexpected invoice output")
	}
	assertPageSize(t, result.Labels, "218 x 360 pts")
	assertPageSize(t, result.Invoices, "595 x 456 pts")
}

func TestPopplerOmitsInvoicesAndRejectsUnknownGeometry(t *testing.T) {
	pdf, err := os.ReadFile("testdata/flipkart_a4.pdf")
	if err != nil {
		t.Fatal(err)
	}
	result, err := pdfgenerator.NewPoppler().Generate(context.Background(), []pdfgenerator.Page{{SourceID: "fixture", PDF: pdf, Number: 1}}, false)
	if err != nil || len(result.Labels) == 0 || result.Invoices != nil {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if _, err = pdfgenerator.NewPoppler().Generate(context.Background(), []pdfgenerator.Page{{SourceID: "bad", PDF: []byte("%PDF-bad"), Number: 1}}, false); err == nil {
		t.Fatal("expected rejection")
	}
}

func TestRepresentativePrivateA4Layouts(t *testing.T) {
	directory := os.Getenv("FLIPKART_PRIVATE_SAMPLES_DIR")
	if directory == "" {
		t.Skip("FLIPKART_PRIVATE_SAMPLES_DIR is not set")
	}
	paths := make([]string, 0)
	for _, pattern := range []string{"invoice_labels_*.pdf", "flipkart_cropped*.pdf"} {
		matches, err := filepath.Glob(filepath.Join(directory, pattern))
		if err != nil || len(matches) == 0 {
			t.Fatalf("private fixture pattern unavailable")
		}
		paths = append(paths, matches...)
	}
	sort.Strings(paths)
	for index, path := range paths {
		pdf, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		result, err := pdfgenerator.NewPoppler().Generate(context.Background(), []pdfgenerator.Page{{SourceID: "fixture", PDF: pdf, Number: 1}}, true)
		if err != nil {
			t.Fatalf("representative file %d: %v", index+1, err)
		}
		assertPageSize(t, result.Labels, "218 x 360 pts")
		assertPageSize(t, result.Invoices, "595 x 456 pts")
	}
}

func assertPageSize(t *testing.T, pdf []byte, expected string) {
	t.Helper()
	file, err := os.CreateTemp("", "commerceops-generated-*.pdf")
	if err != nil {
		t.Fatal(err)
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(pdf); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("pdfinfo", name).CombinedOutput()
	if err != nil || !strings.Contains(string(output), "Page size:       "+expected) {
		t.Fatalf("pdfinfo=%s err=%v", output, err)
	}
}
