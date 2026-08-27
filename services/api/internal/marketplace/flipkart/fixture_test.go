package flipkart

import (
	"context"
	"os"
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
