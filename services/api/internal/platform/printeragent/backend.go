// Package printeragent contains the local hardware bridge. Business decisions
// stay on the server; this package only discovers printers and submits verified
// server-authorized PDFs.
package printeragent

import "context"

type DiscoveredPrinter struct {
	OSPrinterID   string         `json:"os_printer_id"`
	SuggestedName string         `json:"suggested_name"`
	Capabilities  map[string]any `json:"capabilities"`
}

// Backend is the only platform-specific seam. A Windows spooler implementation
// can replace CUPS without changing the server protocol or job semantics.
type Backend interface {
	List(context.Context) ([]DiscoveredPrinter, error)
	SubmitPDF(context.Context, string, int, string) error
}
