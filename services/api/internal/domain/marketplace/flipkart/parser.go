// This file turns marketplace-owned input into validated normalized records used by the domain layer in the Flipkart marketplace adapter.
package flipkart

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
)

const ParserVersion = "flipkart-text-v3"

var (
	ErrUnsupportedDocument = errors.New("document contains no supported Flipkart labels")
	invoiceMarkerRE        = regexp.MustCompile(`(?im)^\s*Tax\s+Invoice\b`)
	awbMarkerRE            = regexp.MustCompile(`(?im)^\s*(?:Flipkart\s+)?(?:AWB(?:\s*No\.?)?|Tracking\s*(?:ID|No\.?))\s*[:#-]?`)
	awbRE                  = regexp.MustCompile(`(?im)^\s*(?:Flipkart\s+)?(?:AWB(?:\s*No\.?)?|Tracking\s*(?:ID|No\.?))\s*[:#-]?\s*([A-Z0-9-]{6,30})\s*$`)
	inlineAWBRE            = regexp.MustCompile(`(?i)\bAWB\s*[:#-]\s*([A-Z0-9-]{6,30})`)
	orderRE                = regexp.MustCompile(`(?i)\b(OD[A-Z0-9]{6,38})\b`)
	skuHeaderRE            = regexp.MustCompile(`(?i)^\s*SKU\s+ID\s*\|\s*Description\s+QTY\s*$`)
	skuRowRE               = regexp.MustCompile(`(?i)^\s*[0-9]+\s+([A-Z0-9._/+\-]{2,80})\s*\|`)
	labeledSKURE           = regexp.MustCompile(`(?im)^\s*(?:SKU|FSN|Seller\s*SKU)\s*[:#-]\s*([A-Z0-9._/+\-]{2,80})\s*$`)
	labeledQuantityRE      = regexp.MustCompile(`(?im)^\s*(?:Qty|Quantity)\s*[:#-]\s*([0-9]{1,5})\s*$`)
	inlineSKURE            = regexp.MustCompile(`(?i)\b(?:SKU|FSN|Seller\s*SKU)\s*[:#-]\s*([A-Z0-9._/+\-]{2,80})`)
	inlineQuantityRE       = regexp.MustCompile(`(?i)\b(?:Qty|Quantity)\s*[:#-]\s*([0-9]{1,5})\b`)
	shippingAddressRE      = regexp.MustCompile(`(?im)^\s*Shipping/Customer\s+Address\s*:`)
)

type Label struct {
	Page              int
	AWB, OrderID, SKU string
	Quantity          *int
}

// Parse consumes already extracted, real PDF pages. It never interprets PDF
// streams or assigns page numbers itself.
func Parse(pages []pdfextractor.Page) ([]Label, error) {
	labels := make([]Label, 0, len(pages))
	for _, page := range pages {
		labelText := shippingLabelText(page.Text)
		if !looksLikeFlipkartLabel(labelText) {
			continue
		}
		sku, quantity := parseSKUAndQuantity(labelText)
		label := Label{
			Page:     page.Number,
			AWB:      extractAWB(labelText),
			OrderID:  capture(orderRE, labelText),
			SKU:      sku,
			Quantity: quantity,
		}
		labels = append(labels, label)
	}
	if len(labels) == 0 {
		return nil, ErrUnsupportedDocument
	}
	return labels, nil
}

func extractAWB(text string) string {
	if value := capture(awbRE, text); value != "" {
		return value
	}
	return capture(inlineAWBRE, text)
}

func shippingLabelText(text string) string {
	if marker := invoiceMarkerRE.FindStringIndex(text); marker != nil {
		return text[:marker[0]]
	}
	return text
}

func looksLikeFlipkartLabel(text string) bool {
	signals := 0
	for _, found := range []bool{
		awbMarkerRE.MatchString(text),
		orderRE.MatchString(text),
		hasSKUHeader(text),
		shippingAddressRE.MatchString(text),
		strings.Contains(strings.ToLower(text), "flipkart"),
	} {
		if found {
			signals++
		}
	}
	return signals >= 3
}

func hasSKUHeader(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		if skuHeaderRE.MatchString(line) {
			return true
		}
	}
	return false
}

func parseSKUAndQuantity(text string) (string, *int) {
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		if !skuHeaderRE.MatchString(line) {
			continue
		}

		quantityColumn := strings.LastIndex(strings.ToLower(line), "qty")
		for _, row := range lines[index+1:] {
			if strings.TrimSpace(row) == "" {
				continue
			}
			match := skuRowRE.FindStringSubmatch(row)
			if len(match) != 2 {
				return "", nil
			}
			return match[1], quantityAtColumn(row, quantityColumn)
		}
		return "", nil
	}

	sku := capture(labeledSKURE, text)
	if sku == "" {
		sku = capture(inlineSKURE, text)
	}
	quantity := capture(labeledQuantityRE, text)
	if quantity == "" {
		quantity = capture(inlineQuantityRE, text)
	}
	return sku, positiveQuantity(quantity)
}

func quantityAtColumn(row string, column int) *int {
	if column < 0 || len(row) <= column {
		return nil
	}
	fields := strings.Fields(row[column:])
	if len(fields) != 1 {
		return nil
	}
	return positiveQuantity(fields[0])
}

func positiveQuantity(raw string) *int {
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return nil
	}
	return &n
}

func capture(re *regexp.Regexp, value string) string {
	match := re.FindStringSubmatch(value)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}
