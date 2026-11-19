// Package config parses command-line flags and environment variables for the server.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
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
	// DefaultEnableHTTPS keeps the server on HTTP unless TLS is explicitly enabled.
	DefaultEnableHTTPS = false
	// DefaultConfigPath disables JSON configuration by default.
	DefaultConfigPath = ""
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
	// EnvEnableHTTPS names the environment variable that enables HTTPS.
	EnvEnableHTTPS = "ENABLE_HTTPS"
	// EnvConfig names the environment variable containing the JSON configuration path.
	EnvConfig = "CONFIG"
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
	// EnableHTTPS enables TLS with a generated self-signed certificate.
	EnableHTTPS bool
}

// Parse combines defaults, JSON, explicitly set flags, and environment variables in increasing priority.
func Parse(args []string) (Config, error) {
	cfg := Config{
		ServerAddress:   DefaultServerAddress,
		BaseURL:         DefaultBaseURL,
		FileStoragePath: DefaultFileStoragePath,
		DatabaseDSN:     DefaultDatabaseDSN,
		AuditFile:       DefaultAuditFile,
		AuditURL:        DefaultAuditURL,
		EnableHTTPS:     DefaultEnableHTTPS,
	}
	flagValues := cfg
	configPath := DefaultConfigPath

	flags := flag.NewFlagSet("shortener", flag.ContinueOnError)
	flags.StringVar(&flagValues.ServerAddress, "a", flagValues.ServerAddress, "HTTP server address")
	flags.StringVar(&flagValues.BaseURL, "b", flagValues.BaseURL, "base URL for shortened links")
	flags.StringVar(&flagValues.FileStoragePath, "f", flagValues.FileStoragePath, "file storage path")
	flags.StringVar(&flagValues.DatabaseDSN, "d", flagValues.DatabaseDSN, "database DSN")
	flags.StringVar(&flagValues.AuditFile, "audit-file", flagValues.AuditFile, "audit log file")
	flags.StringVar(&flagValues.AuditURL, "audit-url", flagValues.AuditURL, "audit server URL")
	flags.BoolVar(&flagValues.EnableHTTPS, "s", flagValues.EnableHTTPS, "enable HTTPS")
	flags.StringVar(&configPath, "c", configPath, "JSON configuration file")
	flags.StringVar(&configPath, "config", configPath, "JSON configuration file")

	if err := flags.Parse(args); err != nil {
		return Config{}, err
	}

	explicitFlags := make(map[string]bool)
	flags.Visit(func(f *flag.Flag) {
		explicitFlags[f.Name] = true
	})

	if value, ok := os.LookupEnv(EnvConfig); ok {
		configPath = value
	}
	if configPath != "" {
		fileValues, err := loadFile(configPath)
		if err != nil {
			return Config{}, err
		}
		applyFile(&cfg, fileValues)
	}
	applyFlags(&cfg, flagValues, explicitFlags)

	if err := applyEnv(&cfg); err != nil {
		return Config{}, err
	}

	if err := validateAuditURL(cfg.AuditURL); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

type fileConfig struct {
	ServerAddress   *string `json:"server_address"`
	BaseURL         *string `json:"base_url"`
	FileStoragePath *string `json:"file_storage_path"`
	DatabaseDSN     *string `json:"database_dsn"`
	EnableHTTPS     *bool   `json:"enable_https"`
	AuditFile       *string `json:"audit_file"`
	AuditURL        *string `json:"audit_url"`
}

func loadFile(path string) (fileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return fileConfig{}, fmt.Errorf("read config file %q: %w", path, err)
	}

	var values fileConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&values); err != nil {
		return fileConfig{}, fmt.Errorf("decode config file %q: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return fileConfig{}, fmt.Errorf("decode config file %q: %w", path, err)
	}

	return values, nil
}

func applyFile(cfg *Config, values fileConfig) {
	if values.ServerAddress != nil {
		cfg.ServerAddress = *values.ServerAddress
	}
	if values.BaseURL != nil {
		cfg.BaseURL = *values.BaseURL
	}
	if values.FileStoragePath != nil {
		cfg.FileStoragePath = *values.FileStoragePath
		cfg.FileStoragePathSet = true
	}
	if values.DatabaseDSN != nil {
		cfg.DatabaseDSN = *values.DatabaseDSN
	}
	if values.EnableHTTPS != nil {
		cfg.EnableHTTPS = *values.EnableHTTPS
	}
	if values.AuditFile != nil {
		cfg.AuditFile = *values.AuditFile
	}
	if values.AuditURL != nil {
		cfg.AuditURL = *values.AuditURL
	}
}

func applyFlags(cfg *Config, values Config, set map[string]bool) {
	if set["a"] {
		cfg.ServerAddress = values.ServerAddress
	}
	if set["b"] {
		cfg.BaseURL = values.BaseURL
	}
	if set["f"] {
		cfg.FileStoragePath = values.FileStoragePath
		cfg.FileStoragePathSet = true
	}
	if set["d"] {
		cfg.DatabaseDSN = values.DatabaseDSN
	}
	if set["s"] {
		cfg.EnableHTTPS = values.EnableHTTPS
	}
	if set["audit-file"] {
		cfg.AuditFile = values.AuditFile
	}
	if set["audit-url"] {
		cfg.AuditURL = values.AuditURL
	}
}

func applyEnv(cfg *Config) error {
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
	if value, ok := os.LookupEnv(EnvEnableHTTPS); ok {
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("parse %s: %w", EnvEnableHTTPS, err)
		}
		cfg.EnableHTTPS = enabled
	}
	return nil
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
