// This file parses Myntra CSV exports and preserves marketplace identifiers needed downstream in the Myntra marketplace adapter.
package myntra

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

const ParserVersion = "myntra-packed-orders-csv-v1"

const (
	maxRows      = 10000
	maxCellBytes = 4096
)

var (
	ErrInvalidCSV     = errors.New("invalid Myntra packed-orders CSV")
	ErrMissingHeaders = errors.New("Myntra CSV is missing required headers")
)

var requiredHeaders = []string{
	"Order id", "Myntra SKU code", "Seller_sku_code", "Store Packet ID",
	"Order_release_id", "Tracking_id", "Status", "Packed On", "Created On",
}

type Record struct {
	Row            int
	OrderID        string
	TrackingID     string
	SellerSKU      string
	MyntraSKU      string
	StorePacketID  string
	OrderReleaseID string
	Status         string
	PackedOn       string
	CreatedOn      string
	Warnings       []string
}

// Parse accepts UTF-8 Myntra packed-orders exports. The row number is retained
// as source traceability; quantity is intentionally absent from this contract.
func Parse(data []byte) ([]Record, error) {
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("%w: input is not UTF-8", ErrInvalidCSV)
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = false
	headers, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("%w: read header: %v", ErrInvalidCSV, err)
	}
	index := make(map[string]int, len(headers))
	for i, header := range headers {
		header = strings.TrimSpace(header)
		if _, exists := index[header]; exists {
			return nil, fmt.Errorf("%w: duplicate header %q", ErrInvalidCSV, header)
		}
		index[header] = i
	}
	for _, header := range requiredHeaders {
		if _, ok := index[header]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrMissingHeaders, header)
		}
	}

	var out []Record
	seenOrder, seenTracking := map[string]struct{}{}, map[string]struct{}{}
	for row := 2; ; row++ {
		values, readErr := r.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("%w: row %d: %v", ErrInvalidCSV, row, readErr)
		}
		if row > maxRows+1 {
			return nil, fmt.Errorf("%w: more than %d data rows", ErrInvalidCSV, maxRows)
		}
		if len(values) != len(headers) {
			return nil, fmt.Errorf("%w: row %d has %d fields, expected %d", ErrInvalidCSV, row, len(values), len(headers))
		}
		for _, cell := range values {
			if len(cell) > maxCellBytes {
				return nil, fmt.Errorf("%w: row %d contains an oversized field", ErrInvalidCSV, row)
			}
		}
		value := func(name string) string { return strings.TrimSpace(values[index[name]]) }
		record := Record{Row: row, OrderID: value("Order id"), TrackingID: value("Tracking_id"), SellerSKU: value("Seller_sku_code"), MyntraSKU: value("Myntra SKU code"), StorePacketID: value("Store Packet ID"), OrderReleaseID: value("Order_release_id"), Status: strings.ToUpper(value("Status")), PackedOn: value("Packed On"), CreatedOn: value("Created On")}
		if record.OrderID == "" {
			record.Warnings = append(record.Warnings, "missing_order_id")
		}
		if record.TrackingID == "" {
			record.Warnings = append(record.Warnings, "missing_awb")
		}
		if record.SellerSKU == "" {
			record.Warnings = append(record.Warnings, "missing_sku")
		}
		if record.MyntraSKU == "" {
			record.Warnings = append(record.Warnings, "missing_myntra_sku")
		}
		if record.StorePacketID == "" {
			record.Warnings = append(record.Warnings, "missing_store_packet_id")
		}
		if record.OrderReleaseID == "" {
			record.Warnings = append(record.Warnings, "missing_order_release_id")
		}
		if record.Status == "" {
			record.Warnings = append(record.Warnings, "missing_marketplace_status")
		}
		if record.PackedOn != "" && !validTimestamp(record.PackedOn) {
			record.Warnings = append(record.Warnings, "malformed_packed_timestamp")
		}
		if record.CreatedOn != "" && !validTimestamp(record.CreatedOn) {
			record.Warnings = append(record.Warnings, "malformed_created_timestamp")
		}
		_, duplicateOrder := seenOrder[record.OrderID]
		_, duplicateTracking := seenTracking[record.TrackingID]
		if record.OrderID != "" && duplicateOrder || record.TrackingID != "" && duplicateTracking {
			record.Warnings = append(record.Warnings, "duplicate_row_identifier")
		}
		seenOrder[record.OrderID], seenTracking[record.TrackingID] = struct{}{}, struct{}{}
		out = append(out, record)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: no data rows", ErrInvalidCSV)
	}
	return out, nil
}

func validTimestamp(value string) bool {
	for _, layout := range []string{"2006-01-02 15:04:05", "02-01-2006 15:04:05", "02/01/2006 15:04:05", time.RFC3339} {
		if _, err := time.Parse(layout, value); err == nil {
			return true
		}
	}
	return false
}
