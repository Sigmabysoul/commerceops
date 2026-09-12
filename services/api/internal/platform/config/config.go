// This file loads and validates environment configuration before the application starts in the configuration package.
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
	HTTPReadTimeout        time.Duration
	HTTPWriteTimeout       time.Duration
	HTTPIdleTimeout        time.Duration
	HTTPMaxHeaderBytes     int
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

	environment := strings.ToLower(valueOrDefault("APP_ENV", "development"))
	if environment != "development" && environment != "test" && environment != "production" {
		return Config{}, fmt.Errorf("APP_ENV must be development, test, or production")
	}
	durations := map[string]time.Duration{}
	for key, fallback := range map[string]string{
		"SHUTDOWN_TIMEOUT": "15s", "DATABASE_TIMEOUT": "2s", "SESSION_LIFETIME": "24h",
		"HTTP_READ_TIMEOUT": "2m", "HTTP_WRITE_TIMEOUT": "5m", "HTTP_IDLE_TIMEOUT": "60s",
	} {
		value, parseErr := time.ParseDuration(valueOrDefault(key, fallback))
		if parseErr != nil || value <= 0 {
			return Config{}, fmt.Errorf("%s must be a positive duration", key)
		}
		durations[key] = value
	}
	maxHeaderBytes, err := strconv.Atoi(valueOrDefault("HTTP_MAX_HEADER_BYTES", "1048576"))
	if err != nil || maxHeaderBytes < 8192 || maxHeaderBytes > 1048576 {
		return Config{}, fmt.Errorf("HTTP_MAX_HEADER_BYTES must be between 8192 and 1048576")
	}

	cfg := Config{
		Environment:            environment,
		HTTPAddr:               valueOrDefault("HTTP_ADDR", ":8080"),
		DatabaseURL:            databaseURL,
		AllowedOrigins:         splitCSV(valueOrDefault("CORS_ALLOWED_ORIGINS", "http://localhost:3000")),
		ShutdownTimeout:        durations["SHUTDOWN_TIMEOUT"],
		DatabaseTimeout:        durations["DATABASE_TIMEOUT"],
		SessionLifetime:        durations["SESSION_LIFETIME"],
		HTTPReadTimeout:        durations["HTTP_READ_TIMEOUT"],
		HTTPWriteTimeout:       durations["HTTP_WRITE_TIMEOUT"],
		HTTPIdleTimeout:        durations["HTTP_IDLE_TIMEOUT"],
		HTTPMaxHeaderBytes:     maxHeaderBytes,
		SecureCookies:          environment != "development",
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
	if err = cfg.validateOrigins(); err != nil {
		return Config{}, err
	}
	if cfg.Environment == "production" && cfg.ObjectStorageDriver != "s3" {
		return Config{}, fmt.Errorf("OBJECT_STORAGE_DRIVER must be s3 in production")
	}
	if cfg.Environment == "production" {
		database, parseErr := url.Parse(cfg.DatabaseURL)
		if parseErr != nil || (database.Scheme != "postgres" && database.Scheme != "postgresql") || database.Host == "" {
			return Config{}, fmt.Errorf("DATABASE_URL must be an absolute PostgreSQL URL in production")
		}
		if database.Query().Get("sslmode") == "disable" {
			return Config{}, fmt.Errorf("DATABASE_URL must not disable TLS in production")
		}
		if cfg.ObjectStorageEndpoint != "" {
			endpoint, _ := url.Parse(cfg.ObjectStorageEndpoint)
			if endpoint.Scheme != "https" {
				return Config{}, fmt.Errorf("OBJECT_STORAGE_ENDPOINT must use HTTPS in production")
			}
		}
	}
	return cfg, nil
}

func (c Config) validateOrigins() error {
	if len(c.AllowedOrigins) == 0 {
		return fmt.Errorf("CORS_ALLOWED_ORIGINS must contain at least one origin")
	}
	seen := make(map[string]struct{}, len(c.AllowedOrigins))
	for _, raw := range c.AllowedOrigins {
		origin, err := url.Parse(raw)
		if err != nil || origin.Host == "" || (origin.Scheme != "http" && origin.Scheme != "https") || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || (origin.Path != "" && origin.Path != "/") {
			return fmt.Errorf("CORS_ALLOWED_ORIGINS contains invalid origin %q", raw)
		}
		if c.Environment == "production" && origin.Scheme != "https" {
			return fmt.Errorf("CORS_ALLOWED_ORIGINS must use HTTPS in production")
		}
		canonical := strings.TrimSuffix(origin.String(), "/")
		if canonical != raw {
			return fmt.Errorf("CORS_ALLOWED_ORIGINS must use canonical origins without a trailing slash")
		}
		if _, duplicate := seen[canonical]; duplicate {
			return fmt.Errorf("CORS_ALLOWED_ORIGINS contains duplicate origin %q", canonical)
		}
		seen[canonical] = struct{}{}
	}
	return nil
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
