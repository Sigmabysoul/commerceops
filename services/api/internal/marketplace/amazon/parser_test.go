package amazon

import (
	"testing"

	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
)

func TestParseExplicitTextLabel(t *testing.T) {
	documents, err := Parse([]pdfextractor.Page{{Number: 7, Text: `amazon.in
Order ID: 406-1234567-1234567
AWB: TRACKING12345
Seller SKU: SELLER-SKU_7
Quantity: 2`}})
	if err != nil || len(documents) != 1 {
		t.Fatalf("documents=%#v err=%v", documents, err)
	}
	document := documents[0]
	if document.Page != 7 || document.OrderID != "406-1234567-1234567" || document.AWB != "TRACKING12345" || document.SKU != "SELLER-SKU_7" || document.Quantity == nil || *document.Quantity != 2 {
		t.Fatalf("document=%#v", document)
	}
}

func TestParseInvoiceSellerSKUAndQuantity(t *testing.T) {
	text := `amazon.in
Tax Invoice/Bill of Supply/Cash Memo
Order Number: 404-1111111-2222222
Description Qty
Sanitized product | B012345678 (                    ₹369.49 3 ₹1,108.47
SELLER-INVOICE-SKU )`
	documents, err := Parse([]pdfextractor.Page{{Number: 4, Text: text}})
	if err != nil || len(documents) != 1 {
		t.Fatalf("documents=%#v err=%v", documents, err)
	}
	document := documents[0]
	if document.Page != 4 || document.OrderID == "" || document.AWB != "" || document.SKU != "SELLER-INVOICE-SKU" || document.Quantity == nil || *document.Quantity != 3 {
		t.Fatalf("document=%#v", document)
	}
}

func TestMissingOrInvalidQuantityNeverDefaults(t *testing.T) {
	for _, quantity := range []string{"", "0", "ambiguous"} {
		documents, err := Parse([]pdfextractor.Page{{Number: 2, Text: "amazon.in\nOrder ID: 406-1234567-7654321\nSeller SKU: SAFE-SKU\nQuantity: " + quantity}})
		if err != nil || documents[0].Quantity != nil {
			t.Fatalf("quantity=%q documents=%#v err=%v", quantity, documents, err)
		}
	}
}

func TestAmbiguousExplicitValuesRemainMissing(t *testing.T) {
	documents, err := Parse([]pdfextractor.Page{{Number: 5, Text: `amazon.in
Order ID: 406-1234567-7654321
Seller SKU: SKU-ONE
Seller SKU: SKU-TWO
Quantity: 1
Quantity: 2`}})
	if err != nil || len(documents) != 1 || documents[0].SKU != "" || documents[0].Quantity != nil {
		t.Fatalf("documents=%#v err=%v", documents, err)
	}
}

func TestRejectsWeakAmazonMentionAndPreservesMissingFields(t *testing.T) {
	if _, err := Parse([]pdfextractor.Page{{Number: 1, Text: "Thank you for shopping at amazon.in"}}); err == nil {
		t.Fatal("weak mention was accepted")
	}
	documents, err := Parse([]pdfextractor.Page{{Number: 9, Text: "amazon.in\nAWB: TRACKONLY123"}})
	if err != nil || len(documents) != 1 || documents[0].Page != 9 || documents[0].OrderID != "" || documents[0].SKU != "" || documents[0].Quantity != nil {
		t.Fatalf("documents=%#v err=%v", documents, err)
	}
}
