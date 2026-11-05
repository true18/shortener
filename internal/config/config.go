// Package config parses command-line flags and environment variables for the server.
package config

import (
	"flag"
	"fmt"
	"net/url"
	"os"
)

const (
	// DefaultServerAddress is the address used when no server address is configured.
	DefaultServerAddress = "localhost:8080"
	// DefaultBaseURL is the public base URL used to build short links.
	DefaultBaseURL = "http://localhost:8080"
	// DefaultFileStoragePath is the conventional file path; file storage is enabled only when explicitly set.
	DefaultFileStoragePath = "storage.json"
	// DefaultDatabaseDSN disables PostgreSQL storage by default.
	DefaultDatabaseDSN = ""
	// DefaultAuditFile disables file audit output by default.
	DefaultAuditFile = ""
	// DefaultAuditURL disables remote audit output by default.
	DefaultAuditURL = ""
	// EnvServerAddress names the environment variable for the HTTP listen address.
	EnvServerAddress = "SERVER_ADDRESS"
	// EnvBaseURL names the environment variable for the public short-link base URL.
	EnvBaseURL = "BASE_URL"
	// EnvFileStoragePath names the environment variable for the file storage path.
	EnvFileStoragePath = "FILE_STORAGE_PATH"
	// EnvDatabaseDSN names the environment variable for the PostgreSQL connection string.
	EnvDatabaseDSN = "DATABASE_DSN"
	// EnvAuditFile names the environment variable for the audit log path.
	EnvAuditFile = "AUDIT_FILE"
	// EnvAuditURL names the environment variable for the remote audit endpoint.
	EnvAuditURL = "AUDIT_URL"
)

// Config contains the server's runtime settings.
type Config struct {
	// ServerAddress is the address on which the HTTP server listens.
	ServerAddress string
	// BaseURL is prepended to generated short IDs in API responses.
	BaseURL string
	// FileStoragePath is the path used by file-backed storage.
	FileStoragePath string
	// FileStoragePathSet records whether the file path was explicitly configured.
	FileStoragePathSet bool
	// DatabaseDSN enables PostgreSQL storage when non-empty.
	DatabaseDSN string
	// AuditFile is an optional JSON Lines output path.
	AuditFile string
	// AuditURL is an optional HTTP endpoint for audit events.
	AuditURL string
}

// Parse combines defaults, command-line flags, and environment variables, with environment variables taking precedence.
func Parse(args []string) (Config, error) {
	cfg := Config{
		ServerAddress:   DefaultServerAddress,
		BaseURL:         DefaultBaseURL,
		FileStoragePath: DefaultFileStoragePath,
		DatabaseDSN:     DefaultDatabaseDSN,
		AuditFile:       DefaultAuditFile,
		AuditURL:        DefaultAuditURL,
	}

	flags := flag.NewFlagSet("shortener", flag.ContinueOnError)
	flags.StringVar(&cfg.ServerAddress, "a", cfg.ServerAddress, "HTTP server address")
	flags.StringVar(&cfg.BaseURL, "b", cfg.BaseURL, "base URL for shortened links")
	flags.StringVar(&cfg.FileStoragePath, "f", cfg.FileStoragePath, "file storage path")
	flags.StringVar(&cfg.DatabaseDSN, "d", cfg.DatabaseDSN, "database DSN")
	flags.StringVar(&cfg.AuditFile, "audit-file", cfg.AuditFile, "audit log file")
	flags.StringVar(&cfg.AuditURL, "audit-url", cfg.AuditURL, "audit server URL")

	if err := flags.Parse(args); err != nil {
		return Config{}, err
	}

	flags.Visit(func(f *flag.Flag) {
		if f.Name == "f" {
			cfg.FileStoragePathSet = true
		}
	})

	if value, ok := os.LookupEnv(EnvServerAddress); ok {
		cfg.ServerAddress = value
	}
	if value, ok := os.LookupEnv(EnvBaseURL); ok {
		cfg.BaseURL = value
	}
	if value, ok := os.LookupEnv(EnvFileStoragePath); ok {
		cfg.FileStoragePath = value
		cfg.FileStoragePathSet = true
	}
	if value, ok := os.LookupEnv(EnvDatabaseDSN); ok {
		cfg.DatabaseDSN = value
	}
	if value, ok := os.LookupEnv(EnvAuditFile); ok {
		cfg.AuditFile = value
	}
	if value, ok := os.LookupEnv(EnvAuditURL); ok {
		cfg.AuditURL = value
	}

	if err := validateAuditURL(cfg.AuditURL); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func validateAuditURL(rawURL string) error {
	if rawURL == "" {
		return nil
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid audit URL: %w", err)
	}
	if parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("invalid audit URL %q: expected an HTTP or HTTPS URL with a host", rawURL)
	}

	return nil
}
