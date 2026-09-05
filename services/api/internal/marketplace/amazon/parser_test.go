// This file exercises parser normalization, validation, and malformed-input edge cases in the Amazon marketplace adapter.
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

func TestAssociatesLabelAndInvoiceByExactOrderID(t *testing.T) {
	pages := []pdfextractor.Page{
		{Number: 8, ExtractionMethod: "text", Text: "amazon.in\nTax Invoice\nOrder Number: 406-5555555-6666666\nSeller SKU: INVOICE-SKU\nQuantity: 4"},
		{Number: 3, ExtractionMethod: "ocr", Text: "amazon.in Shipping Label\nOrder ID: 406-5555555-6666666\nAWB: TRACKASSOC123"},
	}
	documents, err := Parse(pages)
	if err != nil || len(documents) != 1 {
		t.Fatalf("documents=%#v err=%v", documents, err)
	}
	document := documents[0]
	if document.Page != 3 || document.AWB != "TRACKASSOC123" || document.SKU != "INVOICE-SKU" || document.Quantity == nil || *document.Quantity != 4 || len(document.Sources) != 2 || document.Sources[0].ExtractionMethod != "ocr" || document.Sources[1].ExtractionMethod != "text" || document.AssociationMethod != "exact_order_id" || document.Confidence != "high" {
		t.Fatalf("document=%#v", document)
	}
}

func TestInvoiceSKUPrecedenceFromLegacyUsefulPatterns(t *testing.T) {
	tests := []struct{ name, body, expected string }{
		{"bracket directly before HSN", "Description [BRACKET-DIRECT] HSN 4202", "BRACKET-DIRECT"},
		{"bracketed code", "Description (USEFUL_SKU) details HSN 4202", "USEFUL_SKU"},
		{"token before HSN", "Description TOKEN-HSN HSN 4202", "TOKEN-HSN"},
		{"labeled fallback", "Seller SKU: LABELED-SKU", "LABELED-SKU"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			text := "amazon.in\nTax Invoice\nOrder ID: 406-1212121-3434343\n" + test.body + "\nQuantity: 2"
			documents, err := Parse([]pdfextractor.Page{{Number: 2, Text: text}})
			if err != nil || len(documents) != 1 || documents[0].SKU != test.expected {
				t.Fatalf("documents=%#v err=%v", documents, err)
			}
		})
	}
}

func TestAmbiguousHigherPrecedenceSKUNeverFallsThrough(t *testing.T) {
	text := "amazon.in\nTax Invoice\nOrder ID: 406-4545454-6767676\n[SKU-ONE] HSN 4202\n[SKU-TWO] HSN 4202\nSeller SKU: LOWER-FALLBACK\nQuantity: 2"
	documents, err := Parse([]pdfextractor.Page{{Number: 2, Text: text}})
	if err != nil || len(documents) != 1 || documents[0].SKU != "" {
		t.Fatalf("documents=%#v err=%v", documents, err)
	}
}

func TestValidatedAdjacencyFallbackRequiresMutualCompleteEvidence(t *testing.T) {
	pages := []pdfextractor.Page{
		{Number: 1, ExtractionMethod: "ocr", Text: "amazon.in Shipping Label\nAWB: TRACKADJACENT1"},
		{Number: 2, Text: "amazon.in\nTax Invoice\nOrder ID: 406-5656565-7878787\nSeller SKU: ADJ-SKU\nQuantity: 3"},
	}
	documents, err := Parse(pages)
	if err != nil || len(documents) != 1 || documents[0].OrderID == "" || documents[0].AWB != "TRACKADJACENT1" || documents[0].SKU != "ADJ-SKU" || documents[0].Quantity == nil || documents[0].AssociationMethod != "validated_adjacency" || documents[0].Confidence != "medium" {
		t.Fatalf("documents=%#v err=%v", documents, err)
	}

	pages = append(pages, pdfextractor.Page{Number: 3, ExtractionMethod: "ocr", Text: "amazon.in Shipping Label\nAWB: TRACKCOMPETING1"})
	documents, err = Parse(pages)
	if err != nil || len(documents) != 3 {
		t.Fatalf("competing documents=%#v err=%v", documents, err)
	}
	for _, document := range documents {
		if document.AssociationMethod == "validated_adjacency" {
			t.Fatalf("unvalidated fallback=%#v", documents)
		}
	}
}

func TestAmbiguousAssociationRemainsReview(t *testing.T) {
	pages := []pdfextractor.Page{
		{Number: 1, Text: "amazon.in Shipping Label\nOrder ID: 406-7777777-8888888\nAWB: TRACKFIRST123"},
		{Number: 2, Text: "amazon.in Shipping Label\nOrder ID: 406-7777777-8888888\nAWB: TRACKSECOND12"},
		{Number: 3, Text: "amazon.in\nTax Invoice\nOrder ID: 406-7777777-8888888\nSeller SKU: SAFE-SKU\nQuantity: 1"},
	}
	documents, err := Parse(pages)
	if err != nil || len(documents) != 1 || documents[0].AWB != "" || len(documents[0].Warnings) != 1 || documents[0].Warnings[0] != "ambiguous_document_association" {
		t.Fatalf("documents=%#v err=%v", documents, err)
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
