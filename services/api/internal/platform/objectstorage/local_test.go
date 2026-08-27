package objectstorage

import (
	"bytes"
	"context"
	"io"
	"testing"
)

func TestLocalRoundTripAndKeyIsolation(t *testing.T) {
	store, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = store.Put(ctx, "tenant/source.pdf", bytes.NewReader([]byte("pdf")), 3, "application/pdf"); err != nil {
		t.Fatal(err)
	}
	object, err := store.Get(ctx, "tenant/source.pdf")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(object)
	_ = object.Close()
	if string(body) != "pdf" {
		t.Fatalf("body=%q", body)
	}
	for _, key := range []string{"../escape", "/absolute"} {
		if err = store.Put(ctx, key, bytes.NewReader(nil), 0, "application/pdf"); err == nil {
			t.Fatalf("accepted %q", key)
		}
	}
	if err = store.Delete(ctx, "tenant/source.pdf"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Get(ctx, "tenant/source.pdf"); err != ErrNotFound {
		t.Fatalf("expected not found, got %v", err)
	}
}
