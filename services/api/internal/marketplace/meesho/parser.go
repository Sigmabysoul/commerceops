// This file turns marketplace-owned input into validated normalized records used by the domain layer in the Meesho marketplace adapter.
package meesho

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
)

const ParserVersion = "meesho-labeled-v1"

var (
	ErrUnsupportedDocument = errors.New("document contains no supported Meesho labels")
	meeshoRE               = regexp.MustCompile(`(?i)\bmeesho\b`)
	subOrderMarkerRE       = regexp.MustCompile(`(?im)^\s*Sub[ -]?Order\s*(?:No\.?|Number|ID)?\s*[:#-]?`)
	subOrderRE             = regexp.MustCompile(`(?im)^\s*Sub[ -]?Order\s*(?:No\.?|Number|ID)?\s*[:#-]?\s*([A-Z0-9][A-Z0-9_/-]{5,49})\s*$`)
	orderMarkerRE          = regexp.MustCompile(`(?im)^\s*Order\s*(?:No\.?|Number|ID)\s*[:#-]?`)
	orderRE                = regexp.MustCompile(`(?im)^\s*Order\s*(?:No\.?|Number|ID)\s*[:#-]?\s*([A-Z0-9][A-Z0-9_/-]{5,49})\s*$`)
	awbMarkerRE            = regexp.MustCompile(`(?im)^\s*(?:AWB(?:\s*(?:No\.?|Number))?|Tracking\s*(?:ID|No\.?|Number))\s*[:#-]?`)
	awbRE                  = regexp.MustCompile(`(?im)^\s*(?:AWB(?:\s*(?:No\.?|Number))?|Tracking\s*(?:ID|No\.?|Number))\s*[:#-]?\s*([A-Z0-9][A-Z0-9-]{7,39})\s*$`)
	skuMarkerRE            = regexp.MustCompile(`(?im)^\s*(?:Supplier|Seller)?\s*SKU(?:\s*ID)?\s*[:#-]?`)
	skuRE                  = regexp.MustCompile(`(?im)^\s*(?:Supplier|Seller)?\s*SKU(?:\s*ID)?\s*[:#-]?\s*([A-Z0-9][A-Z0-9._/+\-]{1,79})\s*$`)
	quantityMarkerRE       = regexp.MustCompile(`(?im)^\s*(?:Qty|Quantity)\s*[:#-]?`)
	quantityRE             = regexp.MustCompile(`(?im)^\s*(?:Qty|Quantity)\s*[:#-]?\s*([0-9]{1,5})\s*$`)
	shippingAddressRE      = regexp.MustCompile(`(?im)^\s*(?:Shipping|Delivery|Customer)\s+Address\s*:`)
)

type Label struct {
	Page             int
	OrderID          string
	AWB              string
	SKU              string
	Quantity         *int
	ExtractionMethod string
}

// Parse accepts bounded text extracted from PDF pages. It recognizes only
// explicitly labeled values and never infers quantity or page relationships.
func Parse(pages []pdfextractor.Page) ([]Label, error) {
	labels := make([]Label, 0, len(pages))
	for _, page := range pages {
		if !looksLikeMeeshoLabel(page.Text) {
			continue
		}
		method := page.ExtractionMethod
		if method == "" {
			method = "text"
		}
		labels = append(labels, Label{
			Page: page.Number, OrderID: extractOrderID(page.Text),
			AWB: uniqueCapture(awbRE, page.Text), SKU: uniqueCapture(skuRE, page.Text),
			Quantity: positiveQuantity(uniqueCapture(quantityRE, page.Text)), ExtractionMethod: method,
		})
	}
	if len(labels) == 0 {
		return nil, ErrUnsupportedDocument
	}
	return labels, nil
}

func looksLikeMeeshoLabel(text string) bool {
	signals := 0
	for _, found := range []bool{
		meeshoRE.MatchString(text), subOrderMarkerRE.MatchString(text),
		orderMarkerRE.MatchString(text), awbMarkerRE.MatchString(text),
		skuMarkerRE.MatchString(text), quantityMarkerRE.MatchString(text),
		shippingAddressRE.MatchString(text),
	} {
		if found {
			signals++
		}
	}
	return signals >= 3 && (subOrderMarkerRE.MatchString(text) || orderMarkerRE.MatchString(text)) && awbMarkerRE.MatchString(text)
}

func extractOrderID(text string) string {
	values := captureValues(subOrderRE, text)
	if len(values) > 0 {
		return uniqueValue(values)
	}
	return uniqueCapture(orderRE, text)
}

func uniqueCapture(expression *regexp.Regexp, text string) string {
	return uniqueValue(captureValues(expression, text))
}

func captureValues(expression *regexp.Regexp, text string) map[string]struct{} {
	values := map[string]struct{}{}
	for _, match := range expression.FindAllStringSubmatch(text, -1) {
		if len(match) == 2 && strings.TrimSpace(match[1]) != "" {
			values[strings.TrimSpace(match[1])] = struct{}{}
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

func positiveQuantity(raw string) *int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return nil
	}
	return &value
}
