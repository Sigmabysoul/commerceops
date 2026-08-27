package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Environment            string
	HTTPAddr               string
	DatabaseURL            string
	AllowedOrigins         []string
	ShutdownTimeout        time.Duration
	DatabaseTimeout        time.Duration
	SessionLifetime        time.Duration
	SecureCookies          bool
	FileStorageDir         string
	ObjectStorageDriver    string
	ObjectStorageEndpoint  string
	ObjectStorageBucket    string
	ObjectStorageRegion    string
	ObjectStorageAccessKey string
	ObjectStorageSecretKey string
	ObjectStoragePathStyle bool
}

func Load() (Config, error) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	pathStyle, err := strconv.ParseBool(valueOrDefault("OBJECT_STORAGE_PATH_STYLE", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("OBJECT_STORAGE_PATH_STYLE must be true or false: %w", err)
	}

	cfg := Config{
		Environment:            valueOrDefault("APP_ENV", "development"),
		HTTPAddr:               valueOrDefault("HTTP_ADDR", ":8080"),
		DatabaseURL:            databaseURL,
		AllowedOrigins:         splitCSV(valueOrDefault("CORS_ALLOWED_ORIGINS", "http://localhost:3000")),
		ShutdownTimeout:        10 * time.Second,
		DatabaseTimeout:        2 * time.Second,
		SessionLifetime:        24 * time.Hour,
		SecureCookies:          valueOrDefault("APP_ENV", "development") != "development",
		FileStorageDir:         valueOrDefault("FILE_STORAGE_DIR", "./data/uploads"),
		ObjectStorageDriver:    strings.ToLower(valueOrDefault("OBJECT_STORAGE_DRIVER", "local")),
		ObjectStorageEndpoint:  strings.TrimSpace(os.Getenv("OBJECT_STORAGE_ENDPOINT")),
		ObjectStorageBucket:    strings.TrimSpace(os.Getenv("OBJECT_STORAGE_BUCKET")),
		ObjectStorageRegion:    strings.TrimSpace(os.Getenv("OBJECT_STORAGE_REGION")),
		ObjectStorageAccessKey: strings.TrimSpace(os.Getenv("OBJECT_STORAGE_ACCESS_KEY")),
		ObjectStorageSecretKey: strings.TrimSpace(os.Getenv("OBJECT_STORAGE_SECRET_KEY")),
		ObjectStoragePathStyle: pathStyle,
	}
	if err = cfg.validateObjectStorage(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validateObjectStorage() error {
	switch c.ObjectStorageDriver {
	case "local":
		if strings.TrimSpace(c.FileStorageDir) == "" {
			return fmt.Errorf("FILE_STORAGE_DIR is required when OBJECT_STORAGE_DRIVER=local")
		}
		return nil
	case "s3":
		required := []struct {
			name  string
			value string
		}{
			{"OBJECT_STORAGE_BUCKET", c.ObjectStorageBucket},
			{"OBJECT_STORAGE_REGION", c.ObjectStorageRegion},
			{"OBJECT_STORAGE_ACCESS_KEY", c.ObjectStorageAccessKey},
			{"OBJECT_STORAGE_SECRET_KEY", c.ObjectStorageSecretKey},
		}
		for _, setting := range required {
			if strings.TrimSpace(setting.value) == "" {
				return fmt.Errorf("%s is required when OBJECT_STORAGE_DRIVER=s3", setting.name)
			}
		}
		if c.ObjectStorageEndpoint != "" {
			endpoint, err := url.Parse(c.ObjectStorageEndpoint)
			if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" {
				return fmt.Errorf("OBJECT_STORAGE_ENDPOINT must be an absolute HTTP(S) URL")
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported OBJECT_STORAGE_DRIVER %q: expected local or s3", c.ObjectStorageDriver)
	}
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
