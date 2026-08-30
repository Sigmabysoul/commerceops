package amazon

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
)

const ParserVersion = "amazon-text-v1"

var (
	ErrUnsupportedDocument = errors.New("document contains no supported Amazon order pages")
	orderRE                = regexp.MustCompile(`(?i)\b([0-9]{3}-[0-9]{7}-[0-9]{7})\b`)
	awbRE                  = regexp.MustCompile(`(?im)\b(?:AWB|Tracking(?:\s*(?:ID|No\.?))?)\s*[:#-]?\s*([A-Z0-9-]{8,30})\b`)
	labeledSKURE           = regexp.MustCompile(`(?im)^\s*(?:Seller\s*SKU|Merchant\s*SKU|SKU)\s*[:#-]\s*([A-Z0-9._/+\-]{2,80})\s*$`)
	asinSKURE              = regexp.MustCompile(`(?i)\b[A-Z0-9]{10}\s*\(\s*([A-Z0-9._/+\-]{2,80})\s*\)`)
	invoiceSKURE           = regexp.MustCompile(`(?is)\|\s*[A-Z0-9]{10}\s*\([^\n]*\n\s*([A-Z0-9._/+\-]{2,80})\s*\)`)
	labeledQuantityRE      = regexp.MustCompile(`(?im)^\s*(?:Qty|Quantity)\s*[:#-]\s*([0-9]{1,5})\s*$`)
	invoiceQuantityRE      = regexp.MustCompile(`(?:₹|Rs\.?)\s*[0-9,.]+\s+([0-9]{1,5})\s+(?:₹|Rs\.?)\s*[0-9,.]+`)
	invoiceRE              = regexp.MustCompile(`(?im)^\s*(?:Tax\s+Invoice|Invoice\s+(?:Number|Details))\b`)
	amazonRE               = regexp.MustCompile(`(?i)\bamazon(?:\.in)?\b`)
)

type Document struct {
	Page              int
	AWB, OrderID, SKU string
	Quantity          *int
}

// Parse consumes bounded, page-numbered text from the shared extractor. It
// never performs OCR or associates separate label and invoice pages.
func Parse(pages []pdfextractor.Page) ([]Document, error) {
	documents := make([]Document, 0, len(pages))
	for _, page := range pages {
		orderID := uniqueCapture(page.Text, orderRE)
		awb := uniqueCapture(page.Text, awbRE)
		sku := uniqueCapture(page.Text, labeledSKURE, asinSKURE, invoiceSKURE)
		quantity := positiveQuantity(uniqueCapture(page.Text, labeledQuantityRE, invoiceQuantityRE))
		if !looksLikeAmazonPage(page.Text, orderID, awb, sku) {
			continue
		}
		documents = append(documents, Document{Page: page.Number, AWB: awb, OrderID: orderID, SKU: sku, Quantity: quantity})
	}
	if len(documents) == 0 {
		return nil, ErrUnsupportedDocument
	}
	return documents, nil
}

func looksLikeAmazonPage(text, orderID, awb, sku string) bool {
	signals := 0
	for _, found := range []bool{amazonRE.MatchString(text), invoiceRE.MatchString(text), orderID != "", awb != "", sku != ""} {
		if found {
			signals++
		}
	}
	return signals >= 2 && (orderID != "" || awb != "" || sku != "")
}

func positiveQuantity(raw string) *int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return nil
	}
	return &value
}

func uniqueCapture(text string, expressions ...*regexp.Regexp) string {
	values := map[string]struct{}{}
	for _, expression := range expressions {
		for _, match := range expression.FindAllStringSubmatch(text, -1) {
			if len(match) >= 2 && strings.TrimSpace(match[1]) != "" {
				values[strings.TrimSpace(match[1])] = struct{}{}
			}
		}
	}
	if len(values) != 1 {
		return ""
	}
	for value := range values {
		return value
	}
	return ""
}
