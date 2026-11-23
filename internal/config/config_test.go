package config

import (
	"os"
	"testing"
)

func TestParseDefaults(t *testing.T) {
	unsetEnv(t, EnvServerAddress)
	unsetEnv(t, EnvBaseURL)
	unsetEnv(t, EnvFileStoragePath)
	unsetEnv(t, EnvDatabaseDSN)
	unsetEnv(t, EnvAuditFile)
	unsetEnv(t, EnvAuditURL)
	unsetEnv(t, EnvEnableHTTPS)
	unsetEnv(t, EnvTrustedSubnet)
	unsetEnv(t, EnvGRPCAddress)

	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.ServerAddress != DefaultServerAddress {
		t.Fatalf("ServerAddress = %q, want %q", cfg.ServerAddress, DefaultServerAddress)
	}
	if cfg.BaseURL != DefaultBaseURL {
		t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, DefaultBaseURL)
	}
	if cfg.FileStoragePath != DefaultFileStoragePath {
		t.Fatalf("FileStoragePath = %q, want %q", cfg.FileStoragePath, DefaultFileStoragePath)
	}
	if cfg.FileStoragePathSet {
		t.Fatal("FileStoragePathSet = true, want false")
	}
	if cfg.DatabaseDSN != DefaultDatabaseDSN {
		t.Fatalf("DatabaseDSN = %q, want %q", cfg.DatabaseDSN, DefaultDatabaseDSN)
	}
	if cfg.AuditFile != DefaultAuditFile {
		t.Fatalf("AuditFile = %q, want %q", cfg.AuditFile, DefaultAuditFile)
	}
	if cfg.AuditURL != DefaultAuditURL {
		t.Fatalf("AuditURL = %q, want %q", cfg.AuditURL, DefaultAuditURL)
	}
	if cfg.EnableHTTPS != DefaultEnableHTTPS {
		t.Fatalf("EnableHTTPS = %t, want %t", cfg.EnableHTTPS, DefaultEnableHTTPS)
	}
	if cfg.TrustedSubnet != DefaultTrustedSubnet {
		t.Fatalf("TrustedSubnet = %q, want %q", cfg.TrustedSubnet, DefaultTrustedSubnet)
	}
	if DefaultGRPCAddress != "" {
		t.Fatalf("DefaultGRPCAddress = %q, want disabled gRPC", DefaultGRPCAddress)
	}
	if cfg.GRPCAddress != "" {
		t.Fatalf("GRPCAddress = %q, want disabled gRPC", cfg.GRPCAddress)
	}
}

func TestParseGRPCAddressSourcesAndPriority(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  *string
		want string
	}{
		{name: "flag", args: []string{"-g", "127.0.0.1:9000"}, want: "127.0.0.1:9000"},
		{name: "explicit empty", args: []string{"-g", ""}, want: ""},
		{name: "environment", env: stringPointer("127.0.0.1:9001"), want: "127.0.0.1:9001"},
		{name: "environment overrides flag", args: []string{"-g", "127.0.0.1:9000"}, env: stringPointer("127.0.0.1:9001"), want: "127.0.0.1:9001"},
		{name: "empty environment overrides flag", args: []string{"-g", "127.0.0.1:9000"}, env: stringPointer(""), want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unsetEnv(t, EnvGRPCAddress)
			if tt.env != nil {
				t.Setenv(EnvGRPCAddress, *tt.env)
			}
			cfg, err := Parse(tt.args)
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}
			if cfg.GRPCAddress != tt.want {
				t.Fatalf("GRPCAddress = %q, want %q", cfg.GRPCAddress, tt.want)
			}
		})
	}
}

func stringPointer(value string) *string {
	return &value
}

func TestParseTrustedSubnetSourcesAndPriority(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  string
		want string
	}{
		{name: "flag", args: []string{"-t", "192.168.1.0/24"}, want: "192.168.1.0/24"},
		{name: "environment", env: "2001:db8::/32", want: "2001:db8::/32"},
		{name: "environment overrides flag", args: []string{"-t", "192.168.1.0/24"}, env: "10.0.0.0/8", want: "10.0.0.0/8"},
		{name: "explicit empty", args: []string{"-t", ""}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unsetEnv(t, EnvTrustedSubnet)
			if tt.env != "" {
				t.Setenv(EnvTrustedSubnet, tt.env)
			}

			cfg, err := Parse(tt.args)
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}
			if cfg.TrustedSubnet != tt.want {
				t.Fatalf("TrustedSubnet = %q, want %q", cfg.TrustedSubnet, tt.want)
			}
		})
	}
}

func TestParseInvalidTrustedSubnet(t *testing.T) {
	unsetEnv(t, EnvTrustedSubnet)
	if _, err := Parse([]string{"-t", "192.168.1.42"}); err == nil {
		t.Fatal("Parse returned nil error")
	}
}

func TestParseHTTPSFlag(t *testing.T) {
	unsetEnv(t, EnvEnableHTTPS)

	cfg, err := Parse([]string{"-s"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if !cfg.EnableHTTPS {
		t.Fatal("EnableHTTPS = false, want true")
	}
}

func TestParseHTTPSEnv(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "true", want: true},
		{value: "1", want: true},
		{value: "false", want: false},
		{value: "0", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Setenv(EnvEnableHTTPS, tt.value)
			cfg, err := Parse([]string{"-s=true"})
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}
			if cfg.EnableHTTPS != tt.want {
				t.Fatalf("EnableHTTPS = %t, want %t", cfg.EnableHTTPS, tt.want)
			}
		})
	}
}

func TestParseInvalidHTTPSEnv(t *testing.T) {
	t.Setenv(EnvEnableHTTPS, "sometimes")

	if _, err := Parse(nil); err == nil {
		t.Fatal("Parse returned nil error")
	}
}

func TestParseFlags(t *testing.T) {
	unsetEnv(t, EnvServerAddress)
	unsetEnv(t, EnvBaseURL)
	unsetEnv(t, EnvFileStoragePath)
	unsetEnv(t, EnvDatabaseDSN)

	cfg, err := Parse([]string{
		"-a", "localhost:8888",
		"-b", "http://localhost:8000",
		"-f", "/tmp/shortener.json",
		"-d", "postgres://user:pass@localhost:5432/shortener",
	})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.ServerAddress != "localhost:8888" {
		t.Fatalf("ServerAddress = %q, want %q", cfg.ServerAddress, "localhost:8888")
	}
	if cfg.BaseURL != "http://localhost:8000" {
		t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, "http://localhost:8000")
	}
	if cfg.FileStoragePath != "/tmp/shortener.json" {
		t.Fatalf("FileStoragePath = %q, want %q", cfg.FileStoragePath, "/tmp/shortener.json")
	}
	if !cfg.FileStoragePathSet {
		t.Fatal("FileStoragePathSet = false, want true")
	}
	if cfg.DatabaseDSN != "postgres://user:pass@localhost:5432/shortener" {
		t.Fatalf("DatabaseDSN = %q, want %q", cfg.DatabaseDSN, "postgres://user:pass@localhost:5432/shortener")
	}
}

func TestParseEnvOverridesFlags(t *testing.T) {
	t.Setenv(EnvServerAddress, ":9090")
	t.Setenv(EnvBaseURL, "http://example.com")
	t.Setenv(EnvFileStoragePath, "/tmp/env-storage.json")
	t.Setenv(EnvDatabaseDSN, "postgres://env")

	cfg, err := Parse([]string{
		"-a", ":8081",
		"-b", "http://localhost:8081",
		"-f", "/tmp/flag-storage.json",
		"-d", "postgres://flag",
	})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.ServerAddress != ":9090" {
		t.Fatalf("ServerAddress = %q, want %q", cfg.ServerAddress, ":9090")
	}
	if cfg.BaseURL != "http://example.com" {
		t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, "http://example.com")
	}
	if cfg.FileStoragePath != "/tmp/env-storage.json" {
		t.Fatalf("FileStoragePath = %q, want %q", cfg.FileStoragePath, "/tmp/env-storage.json")
	}
	if !cfg.FileStoragePathSet {
		t.Fatal("FileStoragePathSet = false, want true")
	}
	if cfg.DatabaseDSN != "postgres://env" {
		t.Fatalf("DatabaseDSN = %q, want %q", cfg.DatabaseDSN, "postgres://env")
	}
}

func TestParseServerAddressEnv(t *testing.T) {
	t.Setenv(EnvServerAddress, ":9090")
	unsetEnv(t, EnvBaseURL)
	unsetEnv(t, EnvFileStoragePath)
	unsetEnv(t, EnvDatabaseDSN)

	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.ServerAddress != ":9090" {
		t.Fatalf("ServerAddress = %q, want %q", cfg.ServerAddress, ":9090")
	}
	if cfg.BaseURL != DefaultBaseURL {
		t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, DefaultBaseURL)
	}
}

func TestParseBaseURLEnv(t *testing.T) {
	unsetEnv(t, EnvServerAddress)
	t.Setenv(EnvBaseURL, "http://example.com")
	unsetEnv(t, EnvFileStoragePath)
	unsetEnv(t, EnvDatabaseDSN)

	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.ServerAddress != DefaultServerAddress {
		t.Fatalf("ServerAddress = %q, want %q", cfg.ServerAddress, DefaultServerAddress)
	}
	if cfg.BaseURL != "http://example.com" {
		t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, "http://example.com")
	}
}

func TestParseFileStoragePathFlag(t *testing.T) {
	unsetEnv(t, EnvServerAddress)
	unsetEnv(t, EnvBaseURL)
	unsetEnv(t, EnvFileStoragePath)
	unsetEnv(t, EnvDatabaseDSN)

	cfg, err := Parse([]string{"-f", "/tmp/shortener.json"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.FileStoragePath != "/tmp/shortener.json" {
		t.Fatalf("FileStoragePath = %q, want %q", cfg.FileStoragePath, "/tmp/shortener.json")
	}
	if !cfg.FileStoragePathSet {
		t.Fatal("FileStoragePathSet = false, want true")
	}
}

func TestParseFileStoragePathEnv(t *testing.T) {
	unsetEnv(t, EnvServerAddress)
	unsetEnv(t, EnvBaseURL)
	t.Setenv(EnvFileStoragePath, "/tmp/env-storage.json")
	unsetEnv(t, EnvDatabaseDSN)

	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.FileStoragePath != "/tmp/env-storage.json" {
		t.Fatalf("FileStoragePath = %q, want %q", cfg.FileStoragePath, "/tmp/env-storage.json")
	}
	if !cfg.FileStoragePathSet {
		t.Fatal("FileStoragePathSet = false, want true")
	}
}

func TestParseFileStoragePathEnvOverridesFlag(t *testing.T) {
	unsetEnv(t, EnvServerAddress)
	unsetEnv(t, EnvBaseURL)
	t.Setenv(EnvFileStoragePath, "/tmp/env-storage.json")
	unsetEnv(t, EnvDatabaseDSN)

	cfg, err := Parse([]string{"-f", "/tmp/flag-storage.json"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.FileStoragePath != "/tmp/env-storage.json" {
		t.Fatalf("FileStoragePath = %q, want %q", cfg.FileStoragePath, "/tmp/env-storage.json")
	}
	if !cfg.FileStoragePathSet {
		t.Fatal("FileStoragePathSet = false, want true")
	}
}

func TestParseDatabaseDSNFlag(t *testing.T) {
	unsetEnv(t, EnvServerAddress)
	unsetEnv(t, EnvBaseURL)
	unsetEnv(t, EnvFileStoragePath)
	unsetEnv(t, EnvDatabaseDSN)

	cfg, err := Parse([]string{"-d", "postgres://flag"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.DatabaseDSN != "postgres://flag" {
		t.Fatalf("DatabaseDSN = %q, want %q", cfg.DatabaseDSN, "postgres://flag")
	}
}

func TestParseDatabaseDSNEnv(t *testing.T) {
	unsetEnv(t, EnvServerAddress)
	unsetEnv(t, EnvBaseURL)
	unsetEnv(t, EnvFileStoragePath)
	t.Setenv(EnvDatabaseDSN, "postgres://env")

	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.DatabaseDSN != "postgres://env" {
		t.Fatalf("DatabaseDSN = %q, want %q", cfg.DatabaseDSN, "postgres://env")
	}
}

func TestParseDatabaseDSNEnvOverridesFlag(t *testing.T) {
	unsetEnv(t, EnvServerAddress)
	unsetEnv(t, EnvBaseURL)
	unsetEnv(t, EnvFileStoragePath)
	t.Setenv(EnvDatabaseDSN, "postgres://env")

	cfg, err := Parse([]string{"-d", "postgres://flag"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.DatabaseDSN != "postgres://env" {
		t.Fatalf("DatabaseDSN = %q, want %q", cfg.DatabaseDSN, "postgres://env")
	}
}

func TestParseEmptyDatabaseDSN(t *testing.T) {
	unsetEnv(t, EnvServerAddress)
	unsetEnv(t, EnvBaseURL)
	unsetEnv(t, EnvFileStoragePath)
	unsetEnv(t, EnvDatabaseDSN)

	cfg, err := Parse([]string{"-d", ""})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.DatabaseDSN != "" {
		t.Fatalf("DatabaseDSN = %q, want empty", cfg.DatabaseDSN)
	}
}

func TestParseEmptyDatabaseDSNEnvOverridesFlag(t *testing.T) {
	unsetEnv(t, EnvServerAddress)
	unsetEnv(t, EnvBaseURL)
	unsetEnv(t, EnvFileStoragePath)
	t.Setenv(EnvDatabaseDSN, "")

	cfg, err := Parse([]string{"-d", "postgres://flag"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.DatabaseDSN != "" {
		t.Fatalf("DatabaseDSN = %q, want empty", cfg.DatabaseDSN)
	}
}

func TestParseUnknownFlag(t *testing.T) {
	unsetEnv(t, EnvServerAddress)
	unsetEnv(t, EnvBaseURL)
	unsetEnv(t, EnvFileStoragePath)
	unsetEnv(t, EnvDatabaseDSN)

	if _, err := Parse([]string{"-x"}); err == nil {
		t.Fatal("Parse returned nil error")
	}
}

func TestParseAuditFlags(t *testing.T) {
	unsetEnv(t, EnvAuditFile)
	unsetEnv(t, EnvAuditURL)

	cfg, err := Parse([]string{
		"--audit-file", "/tmp/audit.jsonl",
		"--audit-url", "https://audit.example.com/events",
	})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.AuditFile != "/tmp/audit.jsonl" {
		t.Fatalf("AuditFile = %q", cfg.AuditFile)
	}
	if cfg.AuditURL != "https://audit.example.com/events" {
		t.Fatalf("AuditURL = %q", cfg.AuditURL)
	}
}

func TestParseAuditEnv(t *testing.T) {
	t.Setenv(EnvAuditFile, "/tmp/env-audit.jsonl")
	t.Setenv(EnvAuditURL, "http://audit.example.com/events")

	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.AuditFile != "/tmp/env-audit.jsonl" {
		t.Fatalf("AuditFile = %q", cfg.AuditFile)
	}
	if cfg.AuditURL != "http://audit.example.com/events" {
		t.Fatalf("AuditURL = %q", cfg.AuditURL)
	}
}

func TestParseAuditEnvOverridesFlags(t *testing.T) {
	t.Setenv(EnvAuditFile, "/tmp/env-audit.jsonl")
	t.Setenv(EnvAuditURL, "https://env.example.com/events")

	cfg, err := Parse([]string{
		"--audit-file", "/tmp/flag-audit.jsonl",
		"--audit-url", "https://flag.example.com/events",
	})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.AuditFile != "/tmp/env-audit.jsonl" {
		t.Fatalf("AuditFile = %q", cfg.AuditFile)
	}
	if cfg.AuditURL != "https://env.example.com/events" {
		t.Fatalf("AuditURL = %q", cfg.AuditURL)
	}
}

func TestParseInvalidAuditURL(t *testing.T) {
	unsetEnv(t, EnvAuditURL)

	for _, rawURL := range []string{
		"audit.example.com/events",
		"ftp://audit.example.com/events",
		"https:///events",
	} {
		t.Run(rawURL, func(t *testing.T) {
			if _, err := Parse([]string{"--audit-url", rawURL}); err == nil {
				t.Fatal("Parse returned nil error")
			}
		})
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()

	oldValue, ok := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("Unsetenv(%q): %v", key, err)
	}

	t.Cleanup(func() {
		if ok {
			_ = os.Setenv(key, oldValue)
			return
		}
		_ = os.Unsetenv(key)
	})
}
