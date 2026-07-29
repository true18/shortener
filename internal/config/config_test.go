package config

import "testing"

func TestParseDefaults(t *testing.T) {
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
}

func TestParseFlags(t *testing.T) {
	cfg, err := Parse([]string{
		"-a", "localhost:8888",
		"-b", "http://localhost:8000",
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
}

func TestParseUnknownFlag(t *testing.T) {
	if _, err := Parse([]string{"-x"}); err == nil {
		t.Fatal("Parse returned nil error")
	}
}
