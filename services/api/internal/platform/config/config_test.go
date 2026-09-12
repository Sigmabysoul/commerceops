// This file covers configuration defaults, validation, and environment parsing in the configuration package.
package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing DATABASE_URL to fail")
	}
}

func TestLoadParsesAllowedOrigins(t *testing.T) {
	clearStorageEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:3000, http://localhost:3001")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.AllowedOrigins) != 2 {
		t.Fatalf("AllowedOrigins length = %d, want 2", len(cfg.AllowedOrigins))
	}
	if cfg.ObjectStorageDriver != "local" || cfg.FileStorageDir == "" {
		t.Fatalf("unexpected storage config: %#v", cfg)
	}
}

func TestLoadStorageDrivers(t *testing.T) {
	tests := []struct {
		name          string
		env           map[string]string
		wantDriver    string
		wantPathStyle bool
		wantError     string
	}{
		{
			name:       "local defaults",
			wantDriver: "local",
		},
		{
			name: "s3",
			env: map[string]string{
				"OBJECT_STORAGE_DRIVER":     "S3",
				"OBJECT_STORAGE_ENDPOINT":   "https://objects.example.test",
				"OBJECT_STORAGE_BUCKET":     "commerceops",
				"OBJECT_STORAGE_REGION":     "us-east-1",
				"OBJECT_STORAGE_ACCESS_KEY": "test-access-key",
				"OBJECT_STORAGE_SECRET_KEY": "test-secret-key",
				"OBJECT_STORAGE_PATH_STYLE": "true",
			},
			wantDriver:    "s3",
			wantPathStyle: true,
		},
		{
			name: "invalid driver",
			env: map[string]string{
				"OBJECT_STORAGE_DRIVER": "memory",
			},
			wantError: "unsupported OBJECT_STORAGE_DRIVER",
		},
		{
			name: "invalid path style",
			env: map[string]string{
				"OBJECT_STORAGE_PATH_STYLE": "sometimes",
			},
			wantError: "OBJECT_STORAGE_PATH_STYLE must be true or false",
		},
		{
			name: "invalid endpoint",
			env: map[string]string{
				"OBJECT_STORAGE_DRIVER":     "s3",
				"OBJECT_STORAGE_ENDPOINT":   "objects.example.test",
				"OBJECT_STORAGE_BUCKET":     "commerceops",
				"OBJECT_STORAGE_REGION":     "us-east-1",
				"OBJECT_STORAGE_ACCESS_KEY": "test-access-key",
				"OBJECT_STORAGE_SECRET_KEY": "test-secret-key",
			},
			wantError: "OBJECT_STORAGE_ENDPOINT must be an absolute HTTP(S) URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearStorageEnvironment(t)
			t.Setenv("DATABASE_URL", "postgres://example")
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			cfg, err := Load()
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("Load() error = %v, want containing %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.ObjectStorageDriver != tt.wantDriver || cfg.ObjectStoragePathStyle != tt.wantPathStyle {
				t.Fatalf("storage config = %#v", cfg)
			}
		})
	}
}

func TestLoadRequiresEveryS3Setting(t *testing.T) {
	required := map[string]string{
		"OBJECT_STORAGE_BUCKET":     "commerceops",
		"OBJECT_STORAGE_REGION":     "us-east-1",
		"OBJECT_STORAGE_ACCESS_KEY": "test-access-key",
		"OBJECT_STORAGE_SECRET_KEY": "test-secret-key",
	}
	for missing := range required {
		t.Run(missing, func(t *testing.T) {
			clearStorageEnvironment(t)
			t.Setenv("DATABASE_URL", "postgres://example")
			t.Setenv("OBJECT_STORAGE_DRIVER", "s3")
			for key, value := range required {
				if key != missing {
					t.Setenv(key, value)
				}
			}
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), missing+" is required") {
				t.Fatalf("Load() error = %v, want missing %s", err, missing)
			}
		})
	}
}

func TestLocalStorageRequiresDirectory(t *testing.T) {
	err := (Config{ObjectStorageDriver: "local"}).validateObjectStorage()
	if err == nil || !strings.Contains(err.Error(), "FILE_STORAGE_DIR is required") {
		t.Fatalf("validateObjectStorage() error = %v", err)
	}
}

func TestLoadProductionRequiresHTTPSOriginsAndS3(t *testing.T) {
	clearStorageEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://database.example.test/commerceops?sslmode=require")
	t.Setenv("APP_ENV", "production")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://ops.example.test")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("HTTP production origin error = %v", err)
	}

	t.Setenv("CORS_ALLOWED_ORIGINS", "https://ops.example.test")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "must be s3") {
		t.Fatalf("local production storage error = %v", err)
	}

	t.Setenv("OBJECT_STORAGE_DRIVER", "s3")
	t.Setenv("OBJECT_STORAGE_BUCKET", "commerceops")
	t.Setenv("OBJECT_STORAGE_REGION", "us-east-1")
	t.Setenv("OBJECT_STORAGE_ACCESS_KEY", "test-access")
	t.Setenv("OBJECT_STORAGE_SECRET_KEY", "test-secret")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SecureCookies || cfg.Environment != "production" {
		t.Fatalf("production config = %#v", cfg)
	}
	t.Setenv("DATABASE_URL", "postgres://database.example.test/commerceops?sslmode=disable")
	if _, err = Load(); err == nil || !strings.Contains(err.Error(), "must not disable TLS") {
		t.Fatalf("database TLS error = %v", err)
	}
	t.Setenv("DATABASE_URL", "postgres://database.example.test/commerceops?sslmode=require")
	t.Setenv("OBJECT_STORAGE_ENDPOINT", "http://objects.example.test")
	if _, err = Load(); err == nil || !strings.Contains(err.Error(), "must use HTTPS") {
		t.Fatalf("object storage TLS error = %v", err)
	}
}

func TestLoadRejectsInvalidOriginsAndRuntimeLimits(t *testing.T) {
	for _, origin := range []string{"*", "https://example.test/path", "https://example.test/", "https://user@example.test", "https://example.test?query=yes"} {
		t.Run(origin, func(t *testing.T) {
			clearStorageEnvironment(t)
			t.Setenv("DATABASE_URL", "postgres://example")
			t.Setenv("CORS_ALLOWED_ORIGINS", origin)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), "CORS_ALLOWED_ORIGINS") {
				t.Fatalf("origin %q error = %v", origin, err)
			}
		})
	}
	clearStorageEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("HTTP_WRITE_TIMEOUT", "0s")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "HTTP_WRITE_TIMEOUT") {
		t.Fatalf("timeout error = %v", err)
	}
}

func TestLoadParsesRuntimeLimits(t *testing.T) {
	clearStorageEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("SHUTDOWN_TIMEOUT", "20s")
	t.Setenv("DATABASE_TIMEOUT", "3s")
	t.Setenv("SESSION_LIFETIME", "12h")
	t.Setenv("HTTP_READ_TIMEOUT", "90s")
	t.Setenv("HTTP_WRITE_TIMEOUT", "4m")
	t.Setenv("HTTP_IDLE_TIMEOUT", "45s")
	t.Setenv("HTTP_MAX_HEADER_BYTES", "65536")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ShutdownTimeout != 20*time.Second || cfg.DatabaseTimeout != 3*time.Second || cfg.SessionLifetime != 12*time.Hour || cfg.HTTPReadTimeout != 90*time.Second || cfg.HTTPWriteTimeout != 4*time.Minute || cfg.HTTPIdleTimeout != 45*time.Second || cfg.HTTPMaxHeaderBytes != 65536 {
		t.Fatalf("runtime limits = %#v", cfg)
	}
}

func clearStorageEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"OBJECT_STORAGE_DRIVER",
		"OBJECT_STORAGE_ENDPOINT",
		"OBJECT_STORAGE_BUCKET",
		"OBJECT_STORAGE_REGION",
		"OBJECT_STORAGE_ACCESS_KEY",
		"OBJECT_STORAGE_SECRET_KEY",
		"OBJECT_STORAGE_PATH_STYLE",
		"FILE_STORAGE_DIR",
	} {
		t.Setenv(key, "")
	}
}
