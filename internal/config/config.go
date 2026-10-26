package config

import (
	"flag"
	"fmt"
	"net/url"
	"os"
)

const (
	DefaultServerAddress   = "localhost:8080"
	DefaultBaseURL         = "http://localhost:8080"
	DefaultFileStoragePath = "storage.json"
	DefaultDatabaseDSN     = ""
	DefaultAuditFile       = ""
	DefaultAuditURL        = ""
	EnvServerAddress       = "SERVER_ADDRESS"
	EnvBaseURL             = "BASE_URL"
	EnvFileStoragePath     = "FILE_STORAGE_PATH"
	EnvDatabaseDSN         = "DATABASE_DSN"
	EnvAuditFile           = "AUDIT_FILE"
	EnvAuditURL            = "AUDIT_URL"
)

type Config struct {
	ServerAddress      string
	BaseURL            string
	FileStoragePath    string
	FileStoragePathSet bool
	DatabaseDSN        string
	AuditFile          string
	AuditURL           string
}

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
