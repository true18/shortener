package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseConfigFileAllValues(t *testing.T) {
	clearRuntimeEnv(t)
	path := writeConfig(t, `{
		"server_address":"127.0.0.1:9443",
		"base_url":"https://short.example",
		"file_storage_path":"/tmp/shortener.json",
		"database_dsn":"postgres://config",
		"enable_https":true,
		"audit_file":"/tmp/audit.jsonl",
		"audit_url":"https://audit.example/events"
	}`)

	cfg, err := Parse([]string{"-c", path})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.ServerAddress != "127.0.0.1:9443" || cfg.BaseURL != "https://short.example" {
		t.Fatalf("addresses = %q, %q", cfg.ServerAddress, cfg.BaseURL)
	}
	if cfg.FileStoragePath != "/tmp/shortener.json" || !cfg.FileStoragePathSet {
		t.Fatalf("file storage = %q, set = %t", cfg.FileStoragePath, cfg.FileStoragePathSet)
	}
	if cfg.DatabaseDSN != "postgres://config" || !cfg.EnableHTTPS {
		t.Fatalf("database = %q, HTTPS = %t", cfg.DatabaseDSN, cfg.EnableHTTPS)
	}
	if cfg.AuditFile != "/tmp/audit.jsonl" || cfg.AuditURL != "https://audit.example/events" {
		t.Fatalf("audit = %q, %q", cfg.AuditFile, cfg.AuditURL)
	}
}

func TestParseConfigPathSources(t *testing.T) {
	tests := []struct {
		name string
		args func(string) []string
		env  bool
	}{
		{name: "short flag", args: func(path string) []string { return []string{"-c", path} }},
		{name: "long flag", args: func(path string) []string { return []string{"-config", path} }},
		{name: "environment", args: func(string) []string { return nil }, env: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearRuntimeEnv(t)
			path := writeConfig(t, `{"server_address":"127.0.0.1:9000"}`)
			if tt.env {
				t.Setenv(EnvConfig, path)
			}

			cfg, err := Parse(tt.args(path))
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}
			if cfg.ServerAddress != "127.0.0.1:9000" {
				t.Fatalf("ServerAddress = %q", cfg.ServerAddress)
			}
		})
	}
}

func TestParseExplicitFlagsOverrideConfig(t *testing.T) {
	clearRuntimeEnv(t)
	path := writeConfig(t, `{
		"server_address":"json:8080",
		"database_dsn":"postgres://json",
		"enable_https":true,
		"audit_file":"json-audit"
	}`)

	cfg, err := Parse([]string{
		"-c", path,
		"-a", "flag:8080",
		"-d", "",
		"-s=false",
		"--audit-file", "",
	})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.ServerAddress != "flag:8080" || cfg.DatabaseDSN != "" {
		t.Fatalf("flag values not applied: %+v", cfg)
	}
	if cfg.EnableHTTPS {
		t.Fatal("EnableHTTPS = true, want false from -s=false")
	}
	if cfg.AuditFile != "" {
		t.Fatalf("AuditFile = %q, want explicit empty value", cfg.AuditFile)
	}
}

func TestParseEnvironmentOverridesFlagsAndConfig(t *testing.T) {
	clearRuntimeEnv(t)
	path := writeConfig(t, `{
		"server_address":"json:8080",
		"base_url":"http://json",
		"file_storage_path":"json-storage",
		"database_dsn":"postgres://json",
		"enable_https":true,
		"audit_file":"json-audit",
		"audit_url":"https://json.example/events"
	}`)
	t.Setenv(EnvServerAddress, "env:8080")
	t.Setenv(EnvBaseURL, "http://env")
	t.Setenv(EnvFileStoragePath, "env-storage")
	t.Setenv(EnvDatabaseDSN, "postgres://env")
	t.Setenv(EnvEnableHTTPS, "false")
	t.Setenv(EnvAuditFile, "env-audit")
	t.Setenv(EnvAuditURL, "https://env.example/events")

	cfg, err := Parse([]string{
		"-c", path,
		"-a", "flag:8080",
		"-b", "http://flag",
		"-f", "flag-storage",
		"-d", "postgres://flag",
		"-s=true",
		"--audit-file", "flag-audit",
		"--audit-url", "https://flag.example/events",
	})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.ServerAddress != "env:8080" || cfg.BaseURL != "http://env" {
		t.Fatalf("addresses = %q, %q", cfg.ServerAddress, cfg.BaseURL)
	}
	if cfg.FileStoragePath != "env-storage" || !cfg.FileStoragePathSet {
		t.Fatalf("file storage = %q, set = %t", cfg.FileStoragePath, cfg.FileStoragePathSet)
	}
	if cfg.DatabaseDSN != "postgres://env" || cfg.EnableHTTPS {
		t.Fatalf("database = %q, HTTPS = %t", cfg.DatabaseDSN, cfg.EnableHTTPS)
	}
	if cfg.AuditFile != "env-audit" || cfg.AuditURL != "https://env.example/events" {
		t.Fatalf("audit = %q, %q", cfg.AuditFile, cfg.AuditURL)
	}
}

func TestParseConfigEnvironmentOverridesFlagPath(t *testing.T) {
	clearRuntimeEnv(t)
	flagPath := writeConfig(t, `{"server_address":"flag-file:8080"}`)
	envPath := writeConfig(t, `{"server_address":"env-file:8080"}`)
	t.Setenv(EnvConfig, envPath)

	cfg, err := Parse([]string{"-c", flagPath})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.ServerAddress != "env-file:8080" {
		t.Fatalf("ServerAddress = %q", cfg.ServerAddress)
	}
}

func TestParseConfigMissingValuesKeepDefaults(t *testing.T) {
	clearRuntimeEnv(t)
	path := writeConfig(t, `{"enable_https":true}`)

	cfg, err := Parse([]string{"-c", path})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.ServerAddress != DefaultServerAddress || cfg.BaseURL != DefaultBaseURL {
		t.Fatalf("defaults changed: %+v", cfg)
	}
	if cfg.FileStoragePathSet {
		t.Fatal("FileStoragePathSet = true for missing JSON field")
	}
	if !cfg.EnableHTTPS {
		t.Fatal("EnableHTTPS = false, want true")
	}
}

func TestParseConfigExplicitEmptyFilePath(t *testing.T) {
	clearRuntimeEnv(t)
	path := writeConfig(t, `{"file_storage_path":"","database_dsn":"","audit_file":"","audit_url":""}`)

	cfg, err := Parse([]string{"-c", path})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.FileStoragePath != "" || !cfg.FileStoragePathSet {
		t.Fatalf("file storage = %q, set = %t", cfg.FileStoragePath, cfg.FileStoragePathSet)
	}
}

func TestParseConfigErrors(t *testing.T) {
	tests := []struct {
		name string
		path func(*testing.T) string
	}{
		{name: "invalid JSON", path: func(t *testing.T) string { return writeConfig(t, `{`) }},
		{name: "unknown field", path: func(t *testing.T) string { return writeConfig(t, `{"unknown":true}`) }},
		{name: "multiple values", path: func(t *testing.T) string { return writeConfig(t, `{}`+"\n"+`{}`) }},
		{name: "missing file", path: func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing.json") }},
		{name: "unreadable path", path: func(t *testing.T) string { return t.TempDir() }},
		{name: "invalid audit URL", path: func(t *testing.T) string { return writeConfig(t, `{"audit_url":"ftp://example.com"}`) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearRuntimeEnv(t)
			if _, err := Parse([]string{"-c", tt.path(t)}); err == nil {
				t.Fatal("Parse returned nil error")
			}
		})
	}
}

func TestParseInvalidHTTPSEnvOverridesConfig(t *testing.T) {
	clearRuntimeEnv(t)
	path := writeConfig(t, `{"enable_https":true}`)
	t.Setenv(EnvEnableHTTPS, "invalid")

	if _, err := Parse([]string{"-c", path}); err == nil {
		t.Fatal("Parse returned nil error")
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func clearRuntimeEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		EnvServerAddress,
		EnvBaseURL,
		EnvFileStoragePath,
		EnvDatabaseDSN,
		EnvEnableHTTPS,
		EnvAuditFile,
		EnvAuditURL,
		EnvConfig,
	} {
		unsetEnv(t, key)
	}
}

func TestParseConfigReadErrorIncludesPath(t *testing.T) {
	clearRuntimeEnv(t)
	path := filepath.Join(t.TempDir(), "missing.json")

	_, err := Parse([]string{"-config", path})
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("error = %v, want path %q", err, path)
	}
}
