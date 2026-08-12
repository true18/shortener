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
