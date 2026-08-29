package flipkart

import (
	"strings"
	"testing"

	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
)

func TestParsePreservesActualPageAndMissingQuantity(t *testing.T) {
	pages := []pdfextractor.Page{{Number: 4, Text: "packing list"}, {Number: 7, Text: modernLabel("FMPP0000000001", "OD000000000001", "BAG-3", "")}}
	labels, err := Parse(pages)
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != 1 || labels[0].Page != 7 || labels[0].AWB != "FMPP0000000001" || labels[0].SKU != "BAG-3" {
		t.Fatalf("unexpected: %#v", labels)
	}
	if labels[0].Quantity != nil {
		t.Fatal("missing quantity must not default to one")
	}
}
func TestMultiPageUsesExtractorPageNumbers(t *testing.T) {
	pages := []pdfextractor.Page{{Number: 1, Text: modernLabel("FMPC0000000001", "OD000000000001", "ONE", "2")}, {Number: 2, Text: "invoice only"}, {Number: 3, Text: modernLabel("SF00000000001", "OD000000000003", "THREE", "5")}}
	labels, err := Parse(pages)
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != 2 || labels[0].Page != 1 || labels[1].Page != 3 || *labels[1].Quantity != 5 {
		t.Fatalf("unexpected: %#v", labels)
	}
}
func TestRejectsNonFlipkartPages(t *testing.T) {
	if _, err := Parse([]pdfextractor.Page{{Number: 1, Text: "Tax Invoice\nSold via Flipkart Marketplace\nOrder Id: OD999999999999\nInvoice No: INV-999"}}); err == nil {
		t.Fatal("expected rejection")
	}
}

func TestModernLabelIgnoresInvoiceDecoys(t *testing.T) {
	text := modernLabel("FMPP0000000001", "OD000000000001", "SELLER-SKU-3", "2") + `
Tax Invoice      Order Id:                         Invoice No:
Invoice           OD999999999999                   FMPP9999999999
SKU: INVOICE-SKU
Quantity: 99
`
	labels, err := Parse([]pdfextractor.Page{{Number: 9, Text: text}})
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != 1 {
		t.Fatalf("labels=%#v", labels)
	}
	label := labels[0]
	if label.Page != 9 || label.AWB != "FMPP0000000001" || label.OrderID != "OD000000000001" || label.SKU != "SELLER-SKU-3" || label.Quantity == nil || *label.Quantity != 2 {
		t.Fatalf("label=%#v", label)
	}
}

func TestModernLabelAWBVariants(t *testing.T) {
	for _, awb := range []string{"FMPP0000000001", "FMPC0000000001", "SF00000000001", "5965000000000"} {
		t.Run(awb[:2], func(t *testing.T) {
			labels, err := Parse([]pdfextractor.Page{{Number: 1, Text: modernLabel(awb, "OD000000000001", "SKU-ONE", "1")}})
			if err != nil || len(labels) != 1 || labels[0].AWB != awb {
				t.Fatalf("labels=%#v err=%v", labels, err)
			}
		})
	}
}

func TestSKUHeaderIsNotParsedAsID(t *testing.T) {
	labels, err := Parse([]pdfextractor.Page{{Number: 1, Text: modernLabel("FMPP0000000001", "OD000000000001", "real_SKU+3", "3")}})
	if err != nil {
		t.Fatal(err)
	}
	if labels[0].SKU != "real_SKU+3" || labels[0].SKU == "ID" || labels[0].Quantity == nil || *labels[0].Quantity != 3 {
		t.Fatalf("label=%#v", labels[0])
	}
}

func TestAmbiguousTableQuantityRemainsMissing(t *testing.T) {
	text := modernLabel("FMPP0000000001", "OD000000000001", "PACK-OF-3", "")
	labels, err := Parse([]pdfextractor.Page{{Number: 1, Text: text}})
	if err != nil {
		t.Fatal(err)
	}
	if labels[0].SKU != "PACK-OF-3" || labels[0].Quantity != nil {
		t.Fatalf("label=%#v", labels[0])
	}
}

func TestLooksLikeFlipkartLabelRequiresMultipleSignals(t *testing.T) {
	if looksLikeFlipkartLabel("Flipkart Tax Invoice Order Id: OD000000000001") {
		t.Fatal("invoice mention must not be recognized as a shipping label")
	}
	if !looksLikeFlipkartLabel(shippingLabelText(modernLabel("FMPP0000000001", "OD000000000001", "SKU-ONE", "1"))) {
		t.Fatal("modern shipping label was not recognized")
	}
}

func modernLabel(awb, orderID, sku, quantity string) string {
	row := "                                         1 " + sku + " | Sanitized product description"
	if quantity != "" {
		row += strings.Repeat(" ", 100-len(row)) + quantity
	}
	return `                                      Flipkart prepaid shipping label
                                                            ` + orderID + `

                                                                    AWB No. ` + awb + `
                                      Shipping/Customer Address:
                                      Sanitized customer and address

                                                            SKU ID | Description                    QTY
` + row + "\n"
}
