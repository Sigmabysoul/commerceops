package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Environment           string
	HTTPAddr              string
	DatabaseURL           string
	AllowedOrigins        []string
	ShutdownTimeout       time.Duration
	DatabaseTimeout       time.Duration
	SessionLifetime       time.Duration
	SecureCookies         bool
	FileStorageDir        string
	ObjectStorageDriver   string
	ObjectStorageEndpoint string
	ObjectStorageBucket   string
}

func Load() (Config, error) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	return Config{
		Environment:           valueOrDefault("APP_ENV", "development"),
		HTTPAddr:              valueOrDefault("HTTP_ADDR", ":8080"),
		DatabaseURL:           databaseURL,
		AllowedOrigins:        splitCSV(valueOrDefault("CORS_ALLOWED_ORIGINS", "http://localhost:3000")),
		ShutdownTimeout:       10 * time.Second,
		DatabaseTimeout:       2 * time.Second,
		SessionLifetime:       24 * time.Hour,
		SecureCookies:         valueOrDefault("APP_ENV", "development") != "development",
		FileStorageDir:        valueOrDefault("FILE_STORAGE_DIR", "./data/uploads"),
		ObjectStorageDriver:   valueOrDefault("OBJECT_STORAGE_DRIVER", "local"),
		ObjectStorageEndpoint: strings.TrimSpace(os.Getenv("OBJECT_STORAGE_ENDPOINT")),
		ObjectStorageBucket:   strings.TrimSpace(os.Getenv("OBJECT_STORAGE_BUCKET")),
	}, nil
}

func valueOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
