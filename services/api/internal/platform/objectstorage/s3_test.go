// This file tests S3 request construction and error handling without putting storage details in domain code in the object-storage infrastructure layer.
package objectstorage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestS3PutGetDelete(t *testing.T) {
	const objectPath = "/commerceops/tenant/source.pdf"
	var mutex sync.Mutex
	objects := make(map[string][]byte)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			t.Errorf("request is not signed: Authorization=%q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != objectPath {
			t.Errorf("path = %q, want %q", r.URL.Path, objectPath)
		}

		mutex.Lock()
		defer mutex.Unlock()
		switch r.Method {
		case http.MethodPut:
			if got := r.Header.Get("Content-Type"); got != "application/pdf" {
				t.Errorf("Content-Type = %q", got)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read PUT body: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			objects[r.URL.Path] = body
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			body, ok := objects[r.URL.Path]
			if !ok {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`)
				return
			}
			w.Header().Set("Content-Type", "application/pdf")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		case http.MethodDelete:
			delete(objects, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(server.Close)

	store, err := NewS3(context.Background(), S3Options{
		Endpoint:  server.URL,
		Bucket:    "commerceops",
		Region:    "us-east-1",
		AccessKey: "test-access-key",
		SecretKey: "test-secret-key",
		PathStyle: true,
	})
	if err != nil {
		t.Fatalf("NewS3() error = %v", err)
	}

	ctx := context.Background()
	payload := []byte("sanitized-pdf")
	if err = store.Put(ctx, "tenant/source.pdf", bytes.NewReader(payload), int64(len(payload)), "application/pdf"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	object, err := store.Get(ctx, "tenant/source.pdf")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	body, readErr := io.ReadAll(object)
	closeErr := object.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read error = %v, close error = %v", readErr, closeErr)
	}
	if !bytes.Equal(body, payload) {
		t.Fatalf("Get() body = %q, want %q", body, payload)
	}
	if err = store.Delete(ctx, "tenant/source.pdf"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err = store.Get(ctx, "tenant/source.pdf"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() after Delete error = %v, want ErrNotFound", err)
	}
}

func TestNewS3RequiresSettings(t *testing.T) {
	valid := S3Options{
		Endpoint:  "https://objects.example.test",
		Bucket:    "commerceops",
		Region:    "us-east-1",
		AccessKey: "test-access-key",
		SecretKey: "test-secret-key",
	}
	tests := []struct {
		name    string
		options S3Options
	}{
		{"bucket", withS3Option(valid, func(options *S3Options) { options.Bucket = "" })},
		{"region", withS3Option(valid, func(options *S3Options) { options.Region = "" })},
		{"access key", withS3Option(valid, func(options *S3Options) { options.AccessKey = "" })},
		{"secret key", withS3Option(valid, func(options *S3Options) { options.SecretKey = "" })},
		{"endpoint", withS3Option(valid, func(options *S3Options) { options.Endpoint = "not-a-url" })},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewS3(context.Background(), tt.options); err == nil {
				t.Fatalf("NewS3() accepted invalid %s", tt.name)
			}
		})
	}
}

func withS3Option(options S3Options, modify func(*S3Options)) S3Options {
	modify(&options)
	return options
}
