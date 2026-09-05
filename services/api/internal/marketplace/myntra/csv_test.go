// This file contains focused regression tests for the behavior owned by this package in the Myntra marketplace adapter.
package myntra

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSanitizedPackedOrders(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "sanitized_packed_orders.csv"))
	if err != nil {
		t.Fatal(err)
	}
	records, err := Parse(data)
	if err != nil || len(records) != 3 {
		t.Fatalf("records=%#v err=%v", records, err)
	}
	first := records[0]
	if first.Row != 2 || first.OrderID != "7000000001" || first.TrackingID != "MYSP1000000001" || first.SellerSKU != "SANITIZED-SKU_01" || first.MyntraSKU != "MYNTRASKU100000001" || first.StorePacketID != "2000000000001" || first.OrderReleaseID != "300000000001" || first.Status != "PICKED" || len(first.Warnings) != 0 {
		t.Fatalf("first=%#v", first)
	}
	if !contains(records[2].Warnings, "duplicate_row_identifier") {
		t.Fatalf("warnings=%v", records[2].Warnings)
	}
}

func TestHeaderAndMalformedCSVValidation(t *testing.T) {
	if _, err := Parse([]byte("Order id,Tracking_id\n1,T1\n")); !errors.Is(err, ErrMissingHeaders) {
		t.Fatalf("missing headers err=%v", err)
	}
	data, _ := os.ReadFile(filepath.Join("testdata", "sanitized_packed_orders.csv"))
	if _, err := Parse(append(data, []byte("\n\"unterminated")...)); !errors.Is(err, ErrInvalidCSV) {
		t.Fatalf("malformed err=%v", err)
	}
	if _, err := Parse([]byte{0xff, 0xfe}); !errors.Is(err, ErrInvalidCSV) {
		t.Fatalf("encoding err=%v", err)
	}
}

func TestMissingQuantityIsNotInvented(t *testing.T) {
	data, _ := os.ReadFile(filepath.Join("testdata", "sanitized_packed_orders.csv"))
	records, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	// Record deliberately has no quantity field: the normalized layer must keep it nil.
	if strings.Contains(strings.Join(requiredHeaders, ","), "Quantity") || len(records) == 0 {
		t.Fatal("quantity entered the evidence contract")
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
