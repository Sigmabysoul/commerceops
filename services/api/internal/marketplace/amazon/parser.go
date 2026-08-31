package amazon

import (
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
)

const ParserVersion = "amazon-associated-v2"

var (
	ErrUnsupportedDocument = errors.New("document contains no supported Amazon order pages")
	orderRE                = regexp.MustCompile(`(?i)\b([0-9]{3}-[0-9]{7}-[0-9]{7})\b`)
	ocrOrderRE             = regexp.MustCompile(`(?i)\b([0-9]{3})\s*[-—–]\s*([0-9]{7})\s*[-—–]\s*([0-9]{7})\b`)
	awbRE                  = regexp.MustCompile(`(?im)\b(?:AWB|Tracking(?:\s*(?:ID|No\.?))?)\s*[:#-]?\s*([A-Z0-9-]{8,30})\b`)
	ocrAWBRE               = regexp.MustCompile(`(?im)\bAWB\s+([A-Z0-9-]{8,30})\s*:`)
	labeledSKURE           = regexp.MustCompile(`(?im)^\s*(?:Seller\s*SKU|Merchant\s*SKU|SKU)\s*[:#-]\s*([A-Z0-9._/+\-]{2,80})\s*$`)
	asinSKURE              = regexp.MustCompile(`(?i)\b[A-Z0-9]{10}\s*\(\s*([A-Z0-9._/+\-]{2,80})\s*\)`)
	invoiceSKURE           = regexp.MustCompile(`(?is)\|\s*[A-Z0-9]{10}\s*\([^\n]*\n\s*([A-Z0-9._/+\-]{2,80})\s*\)`)
	labeledQuantityRE      = regexp.MustCompile(`(?im)^\s*(?:Qty|Quantity)\s*[:#-]\s*([0-9]{1,5})\s*$`)
	invoiceQuantityRE      = regexp.MustCompile(`(?:₹|Rs\.?)\s*[0-9,.]+\s+([0-9]{1,5})\s+(?:₹|Rs\.?)\s*[0-9,.]+`)
	invoiceRE              = regexp.MustCompile(`(?im)^\s*(?:Tax\s+Invoice|Invoice\s+(?:Number|Details))\b`)
	amazonRE               = regexp.MustCompile(`(?i)\bamazon(?:\.in)?\b`)
)

type SourceDocument struct {
	Page             int
	Role             string
	ExtractionMethod string
}

type Document struct {
	Page              int
	AWB, OrderID, SKU string
	Quantity          *int
	Sources           []SourceDocument
	Warnings          []string
}

type pageDocument struct {
	page                    int
	awb, orderID, sku, role string
	quantity                *int
	method                  string
}

// Parse associates complementary Amazon pages only by an exact order ID. Page
// position is never used; ambiguous groups retain traceability and need review.
func Parse(pages []pdfextractor.Page) ([]Document, error) {
	groups := map[string][]pageDocument{}
	unkeyed := []pageDocument{}
	for _, page := range pages {
		orderID := uniqueOrderID(page.Text)
		awb := uniqueCapture(page.Text, awbRE, ocrAWBRE)
		sku := uniqueCapture(page.Text, labeledSKURE, asinSKURE, invoiceSKURE)
		quantity := positiveQuantity(uniqueCapture(page.Text, labeledQuantityRE, invoiceQuantityRE))
		if !looksLikeAmazonPage(page.Text, orderID, awb, sku) {
			continue
		}
		role := "shipping_label"
		if invoiceRE.MatchString(page.Text) {
			role = "invoice"
		}
		method := page.ExtractionMethod
		if method == "" {
			method = "text"
		}
		doc := pageDocument{page: page.Number, awb: awb, orderID: orderID, sku: sku, quantity: quantity, role: role, method: method}
		if orderID == "" {
			unkeyed = append(unkeyed, doc)
		} else {
			groups[orderID] = append(groups[orderID], doc)
		}
	}
	if len(groups) == 0 && len(unkeyed) == 0 {
		return nil, ErrUnsupportedDocument
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]Document, 0, len(keys)+len(unkeyed))
	for _, key := range keys {
		result = append(result, associate(key, groups[key]))
	}
	for _, item := range unkeyed {
		result = append(result, associate("", []pageDocument{item}))
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Page < result[j].Page })
	return result, nil
}

func uniqueOrderID(text string) string {
	values := map[string]struct{}{}
	for _, match := range orderRE.FindAllStringSubmatch(text, -1) {
		if len(match) == 2 {
			values[match[1]] = struct{}{}
		}
	}
	for _, match := range ocrOrderRE.FindAllStringSubmatch(text, -1) {
		if len(match) == 4 {
			values[match[1]+"-"+match[2]+"-"+match[3]] = struct{}{}
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

func associate(orderID string, pages []pageDocument) Document {
	labels, invoices := []pageDocument{}, []pageDocument{}
	sources := make([]SourceDocument, 0, len(pages))
	for _, page := range pages {
		sources = append(sources, SourceDocument{Page: page.page, Role: page.role, ExtractionMethod: page.method})
		if page.role == "invoice" {
			invoices = append(invoices, page)
		} else {
			labels = append(labels, page)
		}
	}
	primary := pages[0].page
	if len(labels) == 1 {
		primary = labels[0].page
	}
	warnings := []string{}
	if len(labels) > 1 || len(invoices) > 1 {
		warnings = append(warnings, "ambiguous_document_association")
	}
	skuPages, quantityPages := invoices, invoices
	if len(invoices) == 0 && len(labels) == 1 {
		skuPages, quantityPages = labels, labels
	}
	return Document{Page: primary, OrderID: orderID,
		AWB:      uniqueStringField(labels, func(p pageDocument) string { return p.awb }),
		SKU:      uniqueStringField(skuPages, func(p pageDocument) string { return p.sku }),
		Quantity: uniqueQuantity(quantityPages), Sources: sources, Warnings: warnings}
}

func uniqueStringField(pages []pageDocument, field func(pageDocument) string) string {
	values := map[string]struct{}{}
	for _, page := range pages {
		if value := field(page); value != "" {
			values[value] = struct{}{}
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

func uniqueQuantity(pages []pageDocument) *int {
	values := map[int]struct{}{}
	for _, page := range pages {
		if page.quantity != nil {
			values[*page.quantity] = struct{}{}
		}
	}
	if len(values) != 1 {
		return nil
	}
	for value := range values {
		copy := value
		return &copy
	}
	return nil
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
