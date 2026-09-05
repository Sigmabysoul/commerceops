// This file checks assumptions made about representative marketplace fixture documents in the Flipkart marketplace adapter.
package flipkart

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
)

func TestSanitizedMultiPageFixture(t *testing.T) {
	pdf, err := os.ReadFile("testdata/multi_page.pdf")
	if err != nil {
		t.Fatal(err)
	}
	pages, err := pdfextractor.NewPoppler().Extract(context.Background(), pdf)
	if err != nil {
		t.Fatal(err)
	}
	labels, err := Parse(pages)
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != 2 || labels[0].Page != 1 || labels[1].Page != 3 || labels[1].SKU != "MULTI-THREE" {
		t.Fatalf("labels=%#v", labels)
	}
}

func TestSanitizedModernLabelInvoiceFixture(t *testing.T) {
	pdf, err := os.ReadFile("testdata/modern_label_invoice.pdf")
	if err != nil {
		t.Fatal(err)
	}
	pages, err := pdfextractor.NewPoppler().Extract(context.Background(), pdf)
	if err != nil {
		t.Fatal(err)
	}
	labels, err := Parse(pages)
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != 2 {
		t.Fatalf("labels=%#v", labels)
	}
	if labels[0].Page != 1 || labels[0].AWB != "FMPP0000000001" || labels[0].OrderID != "OD000000000001" || labels[0].SKU != "SANITIZED-SKU-3" || labels[0].Quantity == nil || *labels[0].Quantity != 2 {
		t.Fatalf("first label=%#v", labels[0])
	}
	if labels[1].Page != 2 || labels[1].AWB != "5965000000000" || labels[1].OrderID != "OD000000000002" || labels[1].SKU != "sanitized_SKU+4" || labels[1].Quantity != nil {
		t.Fatalf("second label=%#v", labels[1])
	}
}

func TestSanitizedCropBoxFixtureRetainsInvoiceText(t *testing.T) {
	pdf, err := os.ReadFile("testdata/cropbox_label_invoice.pdf")
	if err != nil {
		t.Fatal(err)
	}
	pages, err := pdfextractor.NewPoppler().Extract(context.Background(), pdf)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || !strings.Contains(strings.ToLower(pages[0].Text), "tax invoice") {
		t.Fatalf("fixture no longer demonstrates retained invoice text: pages=%d", len(pages))
	}
	labels, err := Parse(pages)
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != 1 || labels[0].Page != 1 || labels[0].OrderID != "OD000000000001" || labels[0].SKU != "SANITIZED-SKU-3" || labels[0].Quantity == nil || *labels[0].Quantity != 2 {
		t.Fatalf("labels=%#v", labels)
	}
}

func TestRepresentativePrivatePDFs(t *testing.T) {
	directory := os.Getenv("FLIPKART_PRIVATE_SAMPLES_DIR")
	if directory == "" {
		t.Skip("FLIPKART_PRIVATE_SAMPLES_DIR is not set")
	}

	patterns := []string{"invoice_labels_*.pdf", "flipkart_cropped*.pdf"}
	paths := make([]string, 0)
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(directory, pattern))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) == 0 {
			t.Fatalf("no private samples matched %s", pattern)
		}
		paths = append(paths, matches...)
	}
	sort.Strings(paths)

	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			pdf, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			pages, err := pdfextractor.NewPoppler().Extract(context.Background(), pdf)
			if err != nil {
				t.Fatal(err)
			}
			labels, err := Parse(pages)
			if err != nil {
				t.Fatal(err)
			}
			if len(labels) != len(pages) {
				t.Fatalf("parsed %d labels from %d pages", len(labels), len(pages))
			}
			for index, label := range labels {
				if label.Page != pages[index].Number {
					t.Fatalf("label %d source page=%d, want %d", index, label.Page, pages[index].Number)
				}
				if label.AWB == "" {
					t.Fatalf("page %d has no AWB", label.Page)
				}
				if label.OrderID == "" {
					t.Fatalf("page %d has no order ID", label.Page)
				}
				if label.SKU == "" {
					t.Fatalf("page %d has no SKU", label.Page)
				}
				if label.Quantity == nil || *label.Quantity <= 0 {
					t.Fatalf("page %d has no explicit positive quantity", label.Page)
				}
			}
			if strings.HasPrefix(filepath.Base(path), "flipkart_cropped") && !strings.Contains(strings.ToLower(pages[0].Text), "tax invoice") {
				t.Fatal("cropped sample no longer demonstrates retained invoice text")
			}
		})
	}
}
