package flipkart

import (
	"os"
	"testing"
)

func TestParseLabelAndPreserveMissingQuantity(t *testing.T) {
	pdf := []byte("%PDF-1.4\n1 0 obj\n<<>>\nstream\nBT (Flipkart) Tj (AWB: FKA1234567) Tj (Order ID: OD123456) Tj (SKU: BAG-3) Tj ET\nendstream\nendobj\n%%EOF")
	labels, err := Parse(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != 1 || labels[0].AWB != "FKA1234567" || labels[0].OrderID != "OD123456" || labels[0].SKU != "BAG-3" {
		t.Fatalf("unexpected: %#v", labels)
	}
	if labels[0].Quantity != nil {
		t.Fatal("missing quantity must not default to one")
	}
}

func TestSanitizedFixture(t *testing.T) {
	pdf, err := os.ReadFile("testdata/label_text.pdf")
	if err != nil {
		t.Fatal(err)
	}
	labels, err := Parse(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != 1 || labels[0].SKU != "TEST-SKU-3B" || labels[0].Quantity == nil || *labels[0].Quantity != 2 {
		t.Fatalf("unexpected fixture result: %#v", labels)
	}
}

func TestRejectsMalformedAndNonFlipkartPDF(t *testing.T) {
	for _, data := range [][]byte{[]byte("not pdf"), []byte("%PDF-1.4\nstream\n(Amazon) Tj\nendstream\n%%EOF")} {
		if _, err := Parse(data); err == nil {
			t.Fatal("expected rejection")
		}
	}
}
