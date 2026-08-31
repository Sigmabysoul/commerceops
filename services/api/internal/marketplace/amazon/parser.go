package amazon

import (
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
)

const ParserVersion = "amazon-associated-v3"

var (
	ErrUnsupportedDocument = errors.New("document contains no supported Amazon order pages")
	orderRE                = regexp.MustCompile(`(?i)\b([0-9]{3}-[0-9]{7}-[0-9]{7})\b`)
	ocrOrderRE             = regexp.MustCompile(`(?i)\b([0-9]{3})\s*[-—–]\s*([0-9]{7})\s*[-—–]\s*([0-9]{7})\b`)
	awbRE                  = regexp.MustCompile(`(?im)\b(?:AWB|Tracking(?:\s*(?:ID|No\.?))?)\s*[:#-]?\s*([A-Z0-9-]{8,30})\b`)
	ocrAWBRE               = regexp.MustCompile(`(?im)\bAWB\s+([A-Z0-9-]{8,30})\s*:`)
	bracketBeforeHSNRE     = regexp.MustCompile(`(?i)[\[(]\s*([A-Z0-9._/+\-]{2,80})\s*[\])]\s*(?:\|\s*)?HSN\b`)
	bracketedCodeRE        = regexp.MustCompile(`(?i)[\[(]\s*([A-Z0-9._/+\-]{2,80})\s*[\])]`)
	tokenBeforeHSNRE       = regexp.MustCompile(`(?i)\b([A-Z0-9._/+\-]{2,80})\s+(?:\|\s*)?HSN\b`)
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
	Page                          int
	AWB, OrderID, SKU             string
	Quantity                      *int
	Sources                       []SourceDocument
	Warnings                      []string
	AssociationMethod, Confidence string
}

type pageDocument struct {
	page                    int
	awb, orderID, sku, role string
	quantity                *int
	method                  string
}

// Parse associates complementary Amazon pages by exact order ID first. A
// mutually unique adjacent pair is accepted only when one page lacks an order
// ID and the combined label/invoice evidence is otherwise complete.
func Parse(pages []pdfextractor.Page) ([]Document, error) {
	documents := make([]pageDocument, 0, len(pages))
	for _, page := range pages {
		orderID := uniqueOrderID(page.Text)
		awb := uniqueCapture(page.Text, awbRE, ocrAWBRE)
		sku := extractSKU(page.Text)
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
		documents = append(documents, pageDocument{page: page.Number, awb: awb, orderID: orderID, sku: sku, quantity: quantity, role: role, method: method})
	}
	if len(documents) == 0 {
		return nil, ErrUnsupportedDocument
	}

	groups := map[string][]int{}
	unkeyed := []int{}
	for index, document := range documents {
		if document.orderID == "" {
			unkeyed = append(unkeyed, index)
		} else {
			groups[document.orderID] = append(groups[document.orderID], index)
		}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	usedUnkeyed := map[int]bool{}
	result := make([]Document, 0, len(keys)+len(unkeyed))
	for _, key := range keys {
		indices := append([]int(nil), groups[key]...)
		method, confidence := exactAssociation(documents, indices)
		if len(indices) == 1 {
			if candidate, ok := uniqueAdjacencyCandidate(documents, groups, indices[0], unkeyed, usedUnkeyed); ok {
				indices = append(indices, candidate)
				usedUnkeyed[candidate] = true
				method, confidence = "validated_adjacency", "medium"
			}
		}
		result = append(result, associate(key, selected(documents, indices), method, confidence))
	}
	for _, index := range unkeyed {
		if !usedUnkeyed[index] {
			result = append(result, associate("", []pageDocument{documents[index]}, "unassociated", "none"))
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Page < result[j].Page })
	return result, nil
}

func exactAssociation(documents []pageDocument, indices []int) (string, string) {
	if len(indices) == 1 {
		document := documents[indices[0]]
		if document.awb != "" && document.sku != "" && document.quantity != nil {
			return "single_document", "high"
		}
		return "unassociated", "none"
	}
	labels, invoices := 0, 0
	for _, index := range indices {
		if documents[index].role == "invoice" {
			invoices++
		} else {
			labels++
		}
	}
	if labels == 1 && invoices == 1 {
		return "exact_order_id", "high"
	}
	return "ambiguous", "none"
}

func uniqueAdjacencyCandidate(documents []pageDocument, groups map[string][]int, keyed int, unkeyed []int, used map[int]bool) (int, bool) {
	candidates := []int{}
	for _, candidate := range unkeyed {
		if !used[candidate] && validAdjacentPair(documents[keyed], documents[candidate]) {
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) != 1 {
		return 0, false
	}
	candidate := candidates[0]
	reverse := 0
	for _, indices := range groups {
		if len(indices) == 1 && validAdjacentPair(documents[indices[0]], documents[candidate]) {
			reverse++
		}
	}
	return candidate, reverse == 1
}

func validAdjacentPair(first, second pageDocument) bool {
	if first.role == second.role || abs(first.page-second.page) != 1 || (first.orderID == "") == (second.orderID == "") {
		return false
	}
	label, invoice := first, second
	if label.role == "invoice" {
		label, invoice = invoice, label
	}
	return label.awb != "" && invoice.sku != "" && invoice.quantity != nil
}

func selected(documents []pageDocument, indices []int) []pageDocument {
	result := make([]pageDocument, 0, len(indices))
	for _, index := range indices {
		result = append(result, documents[index])
	}
	return result
}

func associate(orderID string, pages []pageDocument, method, confidence string) Document {
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
	sort.Slice(sources, func(i, j int) bool { return sources[i].Page < sources[j].Page })
	primary := pages[0].page
	if len(labels) == 1 {
		primary = labels[0].page
	}
	warnings := []string{}
	if len(labels) > 1 || len(invoices) > 1 {
		warnings = append(warnings, "ambiguous_document_association")
	}
	if method == "unassociated" {
		warnings = append(warnings, "missing_document_association")
	}
	skuPages, quantityPages := invoices, invoices
	if len(invoices) == 0 && len(labels) == 1 {
		skuPages, quantityPages = labels, labels
	}
	return Document{Page: primary, OrderID: orderID,
		AWB:      uniqueStringField(labels, func(p pageDocument) string { return p.awb }),
		SKU:      uniqueStringField(skuPages, func(p pageDocument) string { return p.sku }),
		Quantity: uniqueQuantity(quantityPages), Sources: sources, Warnings: warnings,
		AssociationMethod: method, Confidence: confidence}
}

func extractSKU(text string) string {
	if values := captureValues(text, bracketBeforeHSNRE); len(values) > 0 {
		return uniqueValue(values)
	}
	bracketed := map[string]struct{}{}
	for _, match := range bracketedCodeRE.FindAllStringSubmatch(text, -1) {
		if len(match) == 2 && (strings.Contains(match[1], "_") || strings.Contains(match[1], "-")) {
			bracketed[strings.TrimSpace(match[1])] = struct{}{}
		}
	}
	if len(bracketed) > 0 {
		return uniqueValue(bracketed)
	}
	if values := captureValues(text, tokenBeforeHSNRE); len(values) > 0 {
		return uniqueValue(values)
	}
	return uniqueCapture(text, labeledSKURE, asinSKURE, invoiceSKURE)
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
	return uniqueValue(values)
}

func uniqueStringField(pages []pageDocument, field func(pageDocument) string) string {
	values := map[string]struct{}{}
	for _, page := range pages {
		if value := field(page); value != "" {
			values[value] = struct{}{}
		}
	}
	return uniqueValue(values)
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
	return uniqueValue(captureValues(text, expressions...))
}

func captureValues(text string, expressions ...*regexp.Regexp) map[string]struct{} {
	values := map[string]struct{}{}
	for _, expression := range expressions {
		for _, match := range expression.FindAllStringSubmatch(text, -1) {
			if len(match) >= 2 && strings.TrimSpace(match[1]) != "" {
				values[strings.TrimSpace(match[1])] = struct{}{}
			}
		}
	}
	return values
}

func uniqueValue(values map[string]struct{}) string {
	if len(values) != 1 {
		return ""
	}
	for value := range values {
		return value
	}
	return ""
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
