package marketplace

import (
	"encoding/json"
	"testing"
)

func TestErrorItemJSONContract(t *testing.T) {
	page := 3
	encoded, err := json.Marshal(ErrorItem{Page: &page, Severity: "warning", Code: "MISSING_SKU", Message: "missing sku"})
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err = json.Unmarshal(encoded, &value); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"source_page", "severity", "code", "message"} {
		if _, ok := value[key]; !ok {
			t.Fatalf("missing JSON key %q in %s", key, encoded)
		}
	}
	for _, wrong := range []string{"Page", "Severity", "Code", "Message"} {
		if _, ok := value[wrong]; ok {
			t.Fatalf("unexpected JSON key %q", wrong)
		}
	}
}
