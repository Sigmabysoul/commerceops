package meesho

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
)

func TestParseExplicitMeeshoLabel(t *testing.T) {
	labels, err := Parse([]pdfextractor.Page{{Number: 4, ExtractionMethod: "text", Text: `Meesho
Sub Order No.: 123456789012_1
AWB Number: MEESHOAWB12345
Supplier SKU: SELLER-SKU_4
Quantity: 3
Shipping Address:`}})
	if err != nil || len(labels) != 1 {
		t.Fatalf("labels=%#v err=%v", labels, err)
	}
	label := labels[0]
	if label.Page != 4 || label.OrderID != "123456789012_1" || label.AWB != "MEESHOAWB12345" || label.SKU != "SELLER-SKU_4" || label.Quantity == nil || *label.Quantity != 3 || label.ExtractionMethod != "text" {
		t.Fatalf("label=%#v", label)
	}
}

func TestSubOrderTakesPrecedenceOverOrder(t *testing.T) {
	labels, err := Parse([]pdfextractor.Page{{Number: 1, Text: `Meesho
Order ID: 123456789012
Sub Order ID: 123456789012_2
Tracking ID: MEESHOAWB22222
Seller SKU: SAFE-SKU
Qty: 2`}})
	if err != nil || len(labels) != 1 || labels[0].OrderID != "123456789012_2" {
		t.Fatalf("labels=%#v err=%v", labels, err)
	}
}

func TestMissingInvalidAndAmbiguousValuesRemainUnresolved(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "missing quantity", text: "Meesho\nSub Order No: 123456789013_1\nAWB: MEESHOAWB30001\nSupplier SKU: SAFE-SKU"},
		{name: "zero quantity", text: "Meesho\nSub Order No: 123456789013_2\nAWB: MEESHOAWB30002\nSupplier SKU: SAFE-SKU\nQty: 0"},
		{name: "ambiguous quantity", text: "Meesho\nSub Order No: 123456789013_3\nAWB: MEESHOAWB30003\nSupplier SKU: SAFE-SKU\nQty: 1\nQuantity: 2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			labels, err := Parse([]pdfextractor.Page{{Number: 2, Text: test.text}})
			if err != nil || len(labels) != 1 || labels[0].Quantity != nil {
				t.Fatalf("labels=%#v err=%v", labels, err)
			}
		})
	}

	labels, err := Parse([]pdfextractor.Page{{Number: 3, Text: `Meesho
Sub Order No: 123456789014_1
Sub Order No: 123456789014_2
AWB: MEESHOAWB40001
Supplier SKU: SKU-ONE
Seller SKU: SKU-TWO
Quantity: 1`}})
	if err != nil || len(labels) != 1 || labels[0].OrderID != "" || labels[0].SKU != "" {
		t.Fatalf("ambiguous labels=%#v err=%v", labels, err)
	}
}

func TestRejectsWeakMeeshoMention(t *testing.T) {
	if _, err := Parse([]pdfextractor.Page{{Number: 1, Text: "Thank you for selling on Meesho"}}); err == nil {
		t.Fatal("weak Meesho mention was accepted")
	}
}

func TestSanitizedFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "sanitized_label.txt"))
	if err != nil {
		t.Fatal(err)
	}
	labels, err := Parse([]pdfextractor.Page{{Number: 7, Text: string(data)}})
	if err != nil || len(labels) != 1 || labels[0].Page != 7 || labels[0].OrderID != "987654321098_1" || labels[0].AWB != "SANITIZEDAWB98765" || labels[0].SKU != "SANITIZED-SKU_10" || labels[0].Quantity == nil || *labels[0].Quantity != 2 {
		t.Fatalf("labels=%#v err=%v", labels, err)
	}
}
