package flipkart

import (
	"testing"

	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
)

func TestParsePreservesActualPageAndMissingQuantity(t *testing.T) {
	pages := []pdfextractor.Page{{Number: 4, Text: "packing list"}, {Number: 7, Text: "Flipkart AWB: FKA1234567 Order ID: OD123456 SKU: BAG-3"}}
	labels, err := Parse(pages)
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != 1 || labels[0].Page != 7 || labels[0].AWB != "FKA1234567" || labels[0].SKU != "BAG-3" {
		t.Fatalf("unexpected: %#v", labels)
	}
	if labels[0].Quantity != nil {
		t.Fatal("missing quantity must not default to one")
	}
}
func TestMultiPageUsesExtractorPageNumbers(t *testing.T) {
	pages := []pdfextractor.Page{{Number: 1, Text: "Flipkart AWB: FKT000001 Order ID: OD000001 SKU: ONE Qty: 2"}, {Number: 2, Text: "invoice only"}, {Number: 3, Text: "Flipkart AWB: FKT000003 Order ID: OD000003 SKU: THREE Quantity: 5"}}
	labels, err := Parse(pages)
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != 2 || labels[0].Page != 1 || labels[1].Page != 3 || *labels[1].Quantity != 5 {
		t.Fatalf("unexpected: %#v", labels)
	}
}
func TestRejectsNonFlipkartPages(t *testing.T) {
	if _, err := Parse([]pdfextractor.Page{{Number: 1, Text: "Amazon label"}}); err == nil {
		t.Fatal("expected rejection")
	}
}
