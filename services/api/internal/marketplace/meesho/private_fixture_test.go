package meesho

import (
	"context"
	"os"
	"testing"

	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
)

func TestPrivateMeeshoSample(t *testing.T) {
	path := os.Getenv("MEESHO_PRIVATE_SAMPLE")
	if path == "" {
		t.Skip("MEESHO_PRIVATE_SAMPLE is not set")
	}
	pdf, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pages, err := pdfextractor.NewPoppler().Extract(context.Background(), pdf)
	if err != nil {
		t.Fatal(err)
	}
	labels, err := Parse(pages)
	if err != nil {
		t.Fatal(err)
	}
	complete := 0
	for _, label := range labels {
		if label.Page <= 0 {
			t.Fatalf("source-page traceability missing: %#v", label)
		}
		if label.OrderID != "" && label.AWB != "" && label.SKU != "" && label.Quantity != nil {
			complete++
		}
	}
	if complete == 0 {
		t.Fatalf("no complete Meesho labels in private sample: %#v", labels)
	}
}
