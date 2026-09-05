// This file turns marketplace-owned input into validated normalized records used by the domain layer in the Snapdeal marketplace adapter.
package snapdeal

import (
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
)

const ParserVersion = "snapdeal-packslip-v1"

var (
	ErrUnsupportedDocument = errors.New("document contains no supported Snapdeal pages")
	suborderRE             = regexp.MustCompile(`(?im)^[ \t]*SUBORDER(?:[ \t]+CODE)?[ \t]*:[ \t]*([0-9]{8,20})\b`)
	skuRE                  = regexp.MustCompile(`(?im)^[ \t]*SKU[ \t]+CODE[ \t]*:[ \t]*([A-Z0-9][A-Z0-9._/+\-]{1,79})[ \t]*$`)
	invoiceQuantityRE      = regexp.MustCompile(`(?im)^[^\n]*?\b([0-9]{1,5})[ \t]+[0-9,.]+[ \t]+[0-9,.]+[ \t]+[0-9,.]+[ \t]+[0-9,.]+`)
	shippingOrderLineRE    = regexp.MustCompile(`(?m)^[ \t]*([0-9]{8,20})[ \t]*\|?[ \t]*$`)
	shippingQuantityLineRE = regexp.MustCompile(`(?m)^[ \t]*[A-Z][A-Z0-9 &.'\-/]+[ \t]+[A-Z0-9]{8,20}[ \t]+([0-9]{1,5})[ \t]*$`)
	compactSKURE           = regexp.MustCompile(`(?m)^[ \t]*([A-Z0-9]+_[A-Z0-9._/+\-]{3,79})[ \t]*$`)
	courierAWBRE           = regexp.MustCompile(`(?m)^[ \t]*((?:SF|SD)[A-Z0-9]{8,30})[ \t]*$`)
	invoiceRE              = regexp.MustCompile(`(?is)TAX\s+INVOICE.*INVOICE\s+NUMBER.*SKU\s+CODE.*SUBORDER.*HSN`)
)

type SourceDocument struct {
	Page                   int
	Role, ExtractionMethod string
}
type Document struct {
	Page                          int
	AWB, OrderID, SKU, CompactSKU string
	Quantity                      *int
	Sources                       []SourceDocument
	Warnings                      []string
	AssociationMethod, Confidence string
}
type pageDocument struct {
	page                                        int
	role, method, orderID, awb, sku, compactSKU string
	quantity                                    *int
	quantityInvalid                             bool
}

func Parse(pages []pdfextractor.Page) ([]Document, error) {
	parsed := make([]pageDocument, 0, len(pages))
	for _, page := range pages {
		role := ""
		switch {
		case looksLikeShipping(page.Text):
			role = "shipping_label"
		case invoiceRE.MatchString(page.Text):
			role = "invoice"
		}
		if role == "" {
			continue
		}
		method := page.ExtractionMethod
		if method == "" {
			method = "text"
		}
		p := pageDocument{page: page.Number, role: role, method: method, orderID: uniqueCapture(page.Text, suborderRE)}
		if role == "invoice" {
			p.sku = uniqueCapture(page.Text, skuRE)
			p.quantity, p.quantityInvalid = invoiceQuantity(page.Text)
		} else {
			p.orderID = shippingOrderID(page.Text)
			p.awb = uniqueCapture(page.Text, courierAWBRE)
			p.compactSKU = compactSKU(page.Text, p.orderID)
			p.quantity, p.quantityInvalid = shippingQuantity(page.Text, p.orderID)
		}
		parsed = append(parsed, p)
	}
	if len(parsed) == 0 {
		return nil, ErrUnsupportedDocument
	}
	groups := map[string][]pageDocument{}
	unkeyed := []pageDocument{}
	for _, page := range parsed {
		if page.orderID == "" {
			unkeyed = append(unkeyed, page)
		} else {
			groups[page.orderID] = append(groups[page.orderID], page)
		}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]Document, 0, len(groups)+len(unkeyed))
	for _, key := range keys {
		out = append(out, associate(key, groups[key]))
	}
	for _, page := range unkeyed {
		out = append(out, associate("", []pageDocument{page}))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Page < out[j].Page })
	return out, nil
}

func looksLikeShipping(text string) bool {
	upper := strings.ToUpper(text)
	return strings.Contains(upper, "DELIVERY ADDRESS") && strings.Contains(upper, "SUBORDER CODE") &&
		strings.Contains(upper, "SHIPPED FROM") && strings.Contains(upper, "SNAPDEAL REFERENCE NO")
}

func associate(orderID string, pages []pageDocument) Document {
	labels, invoices := []pageDocument{}, []pageDocument{}
	sources := make([]SourceDocument, 0, len(pages))
	for _, p := range pages {
		sources = append(sources, SourceDocument{p.page, p.role, p.method})
		if p.role == "invoice" {
			invoices = append(invoices, p)
		} else {
			labels = append(labels, p)
		}
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].Page < sources[j].Page })
	primary := pages[0].page
	if len(labels) == 1 {
		primary = labels[0].page
	}
	warnings := []string{}
	method, confidence := "unassociated", "none"
	if len(labels) == 1 && len(invoices) == 1 {
		method, confidence = "exact_suborder", "high"
	} else if len(labels) > 1 || len(invoices) > 1 {
		warnings = append(warnings, "ambiguous_document_association")
	} else {
		warnings = append(warnings, "missing_document_association")
	}
	quantity, conflict := combinedQuantity(labels, invoices)
	if conflict {
		warnings = append(warnings, "conflicting_quantity")
	}
	for _, page := range pages {
		if page.quantityInvalid {
			warnings = append(warnings, "invalid_quantity")
			quantity = nil
			break
		}
	}
	if len(labels) == 1 && len(invoices) == 1 && (labels[0].quantity == nil || invoices[0].quantity == nil) {
		quantity = nil
	}
	return Document{Page: primary, OrderID: orderID, AWB: uniqueString(labels, func(p pageDocument) string { return p.awb }), SKU: uniqueString(invoices, func(p pageDocument) string { return p.sku }), CompactSKU: uniqueString(labels, func(p pageDocument) string { return p.compactSKU }), Quantity: quantity, Sources: sources, Warnings: warnings, AssociationMethod: method, Confidence: confidence}
}

func combinedQuantity(labels, invoices []pageDocument) (*int, bool) {
	values := map[int]struct{}{}
	for _, pages := range [][]pageDocument{labels, invoices} {
		for _, p := range pages {
			if p.quantity != nil {
				values[*p.quantity] = struct{}{}
			}
		}
	}
	if len(values) != 1 {
		return nil, len(values) > 1
	}
	for v := range values {
		x := v
		return &x, false
	}
	return nil, false
}
func uniqueString(pages []pageDocument, field func(pageDocument) string) string {
	v := map[string]struct{}{}
	for _, p := range pages {
		if x := field(p); x != "" {
			v[x] = struct{}{}
		}
	}
	return uniqueValue(v)
}
func uniqueCapture(text string, re *regexp.Regexp) string {
	v := map[string]struct{}{}
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		if len(m) == 2 {
			v[strings.TrimSpace(m[1])] = struct{}{}
		}
	}
	return uniqueValue(v)
}
func uniqueValue(v map[string]struct{}) string {
	if len(v) != 1 {
		return ""
	}
	for x := range v {
		return x
	}
	return ""
}
func positive(raw string) *int {
	n, e := strconv.Atoi(strings.TrimSpace(raw))
	if e != nil || n <= 0 {
		return nil
	}
	return &n
}
func invoiceQuantity(text string) (*int, bool) {
	raw := uniqueCapture(text, invoiceQuantityRE)
	value := positive(raw)
	return value, raw != "" && value == nil
}
func shippingQuantity(text, order string) (*int, bool) {
	raw := uniqueCapture(text, shippingQuantityLineRE)
	value := positive(raw)
	return value, raw != "" && value == nil
}
func shippingOrderID(text string) string {
	values := map[string]struct{}{}
	for _, match := range shippingOrderLineRE.FindAllStringSubmatch(text, -1) {
		if len(match) == 2 {
			values[match[1]] = struct{}{}
		}
	}
	return uniqueValue(values)
}
func compactSKU(text, order string) string {
	v := map[string]struct{}{}
	for _, m := range compactSKURE.FindAllStringSubmatch(text, -1) {
		if len(m) == 2 && !strings.HasPrefix(m[1], "SKU") {
			v[m[1]] = struct{}{}
		}
	}
	return uniqueValue(v)
}
