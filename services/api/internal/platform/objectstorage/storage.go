package objectstorage

import (
	"context"
	"errors"
	"io"
)

var ErrNotFound = errors.New("object not found")

// Storage is the platform boundary for immutable uploaded objects. Marketplace
// services own business metadata; implementations own object persistence.
type Storage interface {
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}
