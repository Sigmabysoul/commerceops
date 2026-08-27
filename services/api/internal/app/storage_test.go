package app

import (
	"context"
	"testing"

	"github.com/commerceops/commerceops/services/api/internal/config"
	"github.com/commerceops/commerceops/services/api/internal/platform/objectstorage"
)

func TestNewObjectStorageSelectsConfiguredDriver(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Config
		want any
	}{
		{
			name: "local",
			cfg: config.Config{
				ObjectStorageDriver: "local",
				FileStorageDir:      t.TempDir(),
			},
			want: (*objectstorage.Local)(nil),
		},
		{
			name: "s3",
			cfg: config.Config{
				ObjectStorageDriver:    "s3",
				ObjectStorageEndpoint:  "http://127.0.0.1:9000",
				ObjectStorageBucket:    "commerceops",
				ObjectStorageRegion:    "us-east-1",
				ObjectStorageAccessKey: "test-access-key",
				ObjectStorageSecretKey: "test-secret-key",
				ObjectStoragePathStyle: true,
			},
			want: (*objectstorage.S3)(nil),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage, err := newObjectStorage(context.Background(), tt.cfg)
			if err != nil {
				t.Fatalf("newObjectStorage() error = %v", err)
			}
			switch tt.want.(type) {
			case *objectstorage.Local:
				if _, ok := storage.(*objectstorage.Local); !ok {
					t.Fatalf("storage type = %T, want *objectstorage.Local", storage)
				}
			case *objectstorage.S3:
				if _, ok := storage.(*objectstorage.S3); !ok {
					t.Fatalf("storage type = %T, want *objectstorage.S3", storage)
				}
			}
		})
	}
}

func TestNewObjectStorageRejectsUnknownDriver(t *testing.T) {
	if _, err := newObjectStorage(context.Background(), config.Config{ObjectStorageDriver: "memory"}); err == nil {
		t.Fatal("expected unknown storage driver to fail")
	}
}
