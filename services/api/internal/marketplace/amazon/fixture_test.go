package amazon

import (
	"context"
	"os"
	"testing"

	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
)

func TestSanitizedAmazonFixture(t *testing.T) {
	pdf, err := os.ReadFile("testdata/sanitized_label_invoice.pdf")
	if err != nil {
		t.Fatal(err)
	}
	pages, err := pdfextractor.NewPoppler().Extract(context.Background(), pdf)
	if err != nil {
		t.Fatal(err)
	}
	documents, err := Parse(pages)
	if err != nil || len(documents) != 2 {
		t.Fatalf("documents=%#v err=%v", documents, err)
	}
	if documents[0].Page != 1 || documents[0].AWB == "" || documents[0].SKU == "" || documents[0].Quantity == nil || len(documents[0].Sources) != 1 {
		t.Fatalf("label=%#v", documents[0])
	}
	if documents[1].Page != 2 || documents[1].OrderID == "" || documents[1].SKU == "" || documents[1].Quantity == nil || len(documents[1].Sources) != 1 {
		t.Fatalf("invoice=%#v", documents[1])
	}
}

func TestPrivateAmazonSampleStructure(t *testing.T) {
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
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 10 || len(documents) != 5 {
		t.Fatalf("pages=%d normalized_documents=%d", len(pages), len(documents))
	}
	for _, document := range documents {
		if document.Page%2 == 0 || document.OrderID == "" || document.SKU == "" || document.Quantity == nil || document.AWB == "" || len(document.Sources) != 2 {
			t.Fatalf("page %d fields: order=%t sku=%t quantity=%t awb=%t sources=%d", document.Page, document.OrderID != "", document.SKU != "", document.Quantity != nil, document.AWB != "", len(document.Sources))
		}
	}
}
