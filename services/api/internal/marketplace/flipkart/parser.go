package flipkart

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
)

const ParserVersion = "flipkart-text-v2"

var (
	ErrUnsupportedDocument = errors.New("document contains no supported Flipkart labels")
	awbRE                  = regexp.MustCompile(`(?i)\b(?:AWB|Tracking\s*(?:ID|No\.?))\s*[:#-]?\s*([A-Z0-9-]{6,30})`)
	orderRE                = regexp.MustCompile(`(?i)\b(?:Order\s*(?:ID|No\.?)|Order)\s*[:#-]?\s*([A-Z0-9-]{5,40})`)
	skuRE                  = regexp.MustCompile(`(?i)\b(?:SKU|FSN|Seller\s*SKU)\s*[:#-]?\s*([A-Z0-9._/-]{2,80})`)
	quantityRE             = regexp.MustCompile(`(?i)\b(?:Qty|Quantity)\s*[:#-]?\s*([0-9]{1,5})\b`)
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
		if !strings.Contains(strings.ToLower(page.Text), "flipkart") {
			continue
		}
		label := Label{Page: page.Number, AWB: capture(awbRE, page.Text), OrderID: capture(orderRE, page.Text), SKU: capture(skuRE, page.Text)}
		if raw := capture(quantityRE, page.Text); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				label.Quantity = &n
			}
		}
		labels = append(labels, label)
	}
	if len(labels) == 0 {
		return nil, ErrUnsupportedDocument
	}
	return labels, nil
}
func capture(re *regexp.Regexp, value string) string {
	match := re.FindStringSubmatch(value)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}
