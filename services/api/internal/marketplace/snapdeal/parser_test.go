package snapdeal

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
)

func TestParseAndAssociateByExactSuborder(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "sanitized_pages.txt"))
	if err != nil {
		t.Fatal(err)
	}
	parts := splitFixture(string(data))
	docs, err := Parse([]pdfextractor.Page{{Number: 1, Text: parts[0]}, {Number: 2, Text: parts[1]}})
	if err != nil || len(docs) != 1 {
		t.Fatalf("docs=%#v err=%v", docs, err)
	}
	d := docs[0]
	if d.Page != 1 || d.OrderID != "88000000001" || d.AWB != "SF0000000001DM" || d.SKU != "9_SAFE-SKU-R1" || d.CompactSKU != "9_SAFESKUR1" || d.Quantity == nil || *d.Quantity != 2 || d.AssociationMethod != "exact_suborder" || len(d.Sources) != 2 {
		t.Fatalf("doc=%#v", d)
	}
}
func TestNoQuantityDefaultAndConflictsReview(t *testing.T) {
	shipping := shippingText("88000000002", "SF0000000002DM", "9_SAFESKUR2", "")
	invoice := invoiceText("88000000002", "9_SAFE-SKU-R2", "")
	docs, err := Parse([]pdfextractor.Page{{Number: 1, Text: shipping}, {Number: 2, Text: invoice}})
	if err != nil || docs[0].Quantity != nil {
		t.Fatalf("docs=%#v err=%v", docs, err)
	}
	docs, err = Parse([]pdfextractor.Page{{Number: 1, Text: shippingText("88000000003", "SF0000000003DM", "9_SAFESKUR3", "1")}, {Number: 2, Text: invoiceText("88000000003", "9_SAFE-SKU-R3", "2")}})
	if err != nil || docs[0].Quantity != nil || !has(docs[0].Warnings, "conflicting_quantity") {
		t.Fatalf("docs=%#v err=%v", docs, err)
	}
	for _, quantity := range []string{"0", "invalid"} {
		docs, err = Parse([]pdfextractor.Page{{Number: 1, Text: shippingText("88000000007", "SF0000000007DM", "9_SAFESKUR7", quantity)}, {Number: 2, Text: invoiceText("88000000007", "9_SAFE-SKU-R7", quantity)}})
		if err != nil || docs[0].Quantity != nil {
			t.Fatalf("quantity=%q docs=%#v err=%v", quantity, docs, err)
		}
	}
}
func TestAmbiguityMismatchAndUnsupportedRemainReview(t *testing.T) {
	pages := []pdfextractor.Page{{Number: 1, Text: shippingText("88000000004", "SF0000000004DM", "9_SAFESKUR4", "1")}, {Number: 2, Text: invoiceText("88000000005", "9_SAFE-SKU-R4", "1")}}
	docs, err := Parse(pages)
	if err != nil || len(docs) != 2 || docs[0].AssociationMethod != "unassociated" || docs[1].AssociationMethod != "unassociated" {
		t.Fatalf("docs=%#v err=%v", docs, err)
	}
	pages = append(pages[:1], pdfextractor.Page{Number: 3, Text: shippingText("88000000004", "SF0000000006DM", "9_OTHER", "1")})
	docs, err = Parse(pages)
	if err != nil || !has(docs[0].Warnings, "ambiguous_document_association") {
		t.Fatalf("docs=%#v err=%v", docs, err)
	}
	if _, err = Parse([]pdfextractor.Page{{Number: 1, Text: "snapdeal mention only"}}); !errors.Is(err, ErrUnsupportedDocument) {
		t.Fatalf("err=%v", err)
	}
}
func TestAmbiguousSKUIsMissing(t *testing.T) {
	text := invoiceText("88000000006", "9_SAFE-SKU-R6", "1") + "\nSKU CODE: 9_OTHER"
	docs, err := Parse([]pdfextractor.Page{{Number: 2, Text: text}})
	if err != nil || docs[0].SKU != "" {
		t.Fatalf("docs=%#v err=%v", docs, err)
	}
}
func shippingText(order, awb, compact, qty string) string {
	return "snapdeal\nSHADOWFAX\n" + awb + "\nDELIVERY ADDRESS\nSanitized\nSUBORDER CODE SELLER GSTIN QUANTITY\n" + order + " |\nSAFE SELLER 19SAFE0000A1AA " + qty + "\n" + compact + "\nSnapdeal Reference No\nSHIPPED FROM\nSanitized"
}
func invoiceText(order, sku, qty string) string {
	return "TAX INVOICE\nINVOICE NUMBER : SAFE/1\nITEM DESCRIPTION QTY RATE TOTAL DISC TAXABLE VALUE\nSanitized item\n " + qty + " 100 100 0 100\nSKU CODE: " + sku + "\nSUBORDER : " + order + "\nHSN: 0000"
}
func splitFixture(s string) []string {
	const marker = "\n---PAGE---\n"
	for i := 0; i+len(marker) <= len(s); i++ {
		if s[i:i+len(marker)] == marker {
			return []string{s[:i], s[i+len(marker):]}
		}
	}
	return []string{s}
}
func has(v []string, w string) bool {
	for _, x := range v {
		if x == w {
			return true
		}
	}
	return false
}
