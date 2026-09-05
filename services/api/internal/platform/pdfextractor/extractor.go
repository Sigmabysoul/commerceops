// This file defines the PDF text-extraction contract consumed by marketplace processors in the PDF text-extraction infrastructure layer.
package pdfextractor

import "context"

const (
	MaxPages             = 100
	MaxPageTextBytes     = 1 << 20
	MaxDocumentTextBytes = 10 << 20
)

type Page struct {
	Number           int
	Text             string
	ExtractionMethod string
}
type Extractor interface {
	Extract(ctx context.Context, pdf []byte) ([]Page, error)
}
