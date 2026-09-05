// This file tests the Poppler adapter's command behavior and failure reporting in the PDF text-extraction infrastructure layer.
package pdfextractor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPopplerReturnsRealPages(t *testing.T) {
	fixture := filepath.Join("..", "..", "marketplace", "flipkart", "testdata", "multi_page.pdf")
	pdf, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	pages, err := NewPoppler().Extract(context.Background(), pdf)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 3 || pages[0].Number != 1 || pages[1].Number != 2 || pages[2].Number != 3 {
		t.Fatalf("pages=%#v", pages)
	}
	if pages[1].Text == pages[2].Text {
		t.Fatal("page text was not separated")
	}
}
