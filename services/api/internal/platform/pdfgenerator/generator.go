package pdfgenerator

import (
	"context"
	"errors"
)

var ErrUnsupportedLayout = errors.New("unsupported PDF layout")

type Page struct {
	SourceID      string
	PDF           []byte
	Number        int
	InvoiceNumber int
	SKU           string
	Quantity      int
}

type Result struct {
	Labels   []byte
	Invoices []byte
}

type Generator interface {
	Generate(context.Context, []Page, bool) (Result, error)
}
