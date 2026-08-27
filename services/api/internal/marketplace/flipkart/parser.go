package flipkart

import (
	"bytes"
	"compress/flate"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
)

const ParserVersion = "flipkart-pdf-v1"

var (
	errInvalidPDF = errors.New("invalid or unsupported PDF")
	streamRE      = regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`)
	textRE        = regexp.MustCompile(`\((?:\\.|[^\\)])*\)\s*Tj|\[(.*?)\]\s*TJ`)
	parenRE       = regexp.MustCompile(`\((?:\\.|[^\\)])*\)`)
	awbRE         = regexp.MustCompile(`(?i)\b(?:AWB|Tracking\s*(?:ID|No\.?))\s*[:#-]?\s*([A-Z0-9-]{6,30})`)
	orderRE       = regexp.MustCompile(`(?i)\b(?:Order\s*(?:ID|No\.?)|Order)\s*[:#-]?\s*([A-Z0-9-]{5,40})`)
	skuRE         = regexp.MustCompile(`(?i)\b(?:SKU|FSN|Seller\s*SKU)\s*[:#-]?\s*([A-Z0-9._/-]{2,80})`)
	quantityRE    = regexp.MustCompile(`(?i)\b(?:Qty|Quantity)\s*[:#-]?\s*([0-9]{1,5})\b`)
)

type Label struct {
	Page              int
	AWB, OrderID, SKU string
	Quantity          *int
	RawText           string
}

func Parse(data []byte) ([]Label, error) {
	if len(data) < 8 || !bytes.HasPrefix(data, []byte("%PDF-")) || !bytes.Contains(data, []byte("%%EOF")) {
		return nil, errInvalidPDF
	}
	streams := streamRE.FindAllSubmatch(data, -1)
	labels := make([]Label, 0, len(streams))
	for _, match := range streams {
		body := match[1]
		if decoded, err := inflate(body); err == nil {
			body = decoded
		}
		text := extractText(body)
		if !strings.Contains(strings.ToLower(text), "flipkart") {
			continue
		}
		label := Label{Page: len(labels) + 1, RawText: text}
		label.AWB = capture(awbRE, text)
		label.OrderID = capture(orderRE, text)
		label.SKU = capture(skuRE, text)
		if raw := capture(quantityRE, text); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				label.Quantity = &n
			}
		}
		labels = append(labels, label)
	}
	if len(labels) == 0 {
		return nil, errInvalidPDF
	}
	return labels, nil
}

func inflate(body []byte) ([]byte, error) {
	// PDF FlateDecode streams use a zlib wrapper: two header bytes, a deflate
	// payload, and a four-byte Adler-32 checksum.
	if len(body) < 7 {
		return nil, errors.New("short flate stream")
	}
	r := flate.NewReader(bytes.NewReader(body[2 : len(body)-4]))
	defer r.Close()
	return io.ReadAll(r)
}
func capture(re *regexp.Regexp, value string) string {
	match := re.FindStringSubmatch(value)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}
func extractText(body []byte) string {
	parts := make([]string, 0)
	for _, token := range textRE.FindAll(body, -1) {
		for _, value := range parenRE.FindAll(token, -1) {
			raw := string(value[1 : len(value)-1])
			raw = strings.NewReplacer(`\(`, "(", `\)`, ")", `\\`, `\`, `\n`, " ", `\r`, " ").Replace(raw)
			parts = append(parts, raw)
		}
	}
	return strings.Join(parts, " ")
}
