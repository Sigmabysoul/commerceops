// This file checks optional private CSV evidence without making customer order data a source-code dependency in the Myntra adapter.
package myntra

import (
	"bytes"
	"encoding/csv"
	"os"
	"strings"
	"testing"
)

func TestPrivatePackedOrdersCSV(t *testing.T) {
	path := os.Getenv("MYNTRA_PRIVATE_PACKED_ORDERS_CSV")
	if path == "" {
		t.Skip("MYNTRA_PRIVATE_PACKED_ORDERS_CSV is not set")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	records, err := Parse(data)
	if err != nil {
		t.Fatalf("parse private packed-orders CSV: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("private packed-orders CSV contains no records")
	}
	for index, record := range records {
		if record.Row != index+2 {
			t.Fatalf("record %d source row=%d, want %d", index, record.Row, index+2)
		}
	}

	reader := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})))
	headers, err := reader.Read()
	if err != nil {
		t.Fatalf("read private packed-orders headers: %v", err)
	}
	for _, header := range headers {
		switch strings.ToLower(strings.TrimSpace(header)) {
		case "quantity", "qty":
			t.Fatalf("private packed-orders CSV now contains a %q column; review the evidence contract before using it", header)
		}
	}
}
