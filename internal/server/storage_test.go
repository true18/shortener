package server

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/true18/shortener/internal/config"
)

func TestStorageFor(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Config
		want storageKind
	}{
		{
			name: "postgres",
			cfg: config.Config{
				DatabaseDSN:        "postgres://user:pass@localhost:5432/shortener",
				FileStoragePath:    "/tmp/storage.json",
				FileStoragePathSet: true,
			},
			want: storagePostgres,
		},
		{
			name: "file",
			cfg: config.Config{
				FileStoragePath:    "/tmp/storage.json",
				FileStoragePathSet: true,
			},
			want: storageFile,
		},
		{
			name: "memory",
			cfg: config.Config{
				FileStoragePath: config.DefaultFileStoragePath,
			},
			want: storageMemory,
		},
		{
			name: "empty dsn",
			cfg: config.Config{
				DatabaseDSN: "",
			},
			want: storageMemory,
		},
		{
			name: "empty file path",
			cfg: config.Config{
				FileStoragePathSet: true,
			},
			want: storageMemory,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := storageFor(tt.cfg); got != tt.want {
				t.Fatalf("storageFor() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewPostgresStorageWithoutDatabase(t *testing.T) {
	_, err := newStore(config.Config{DatabaseDSN: "postgres://localhost/shortener"}, nil)
	if !errors.Is(err, errDatabaseNotOpen) {
		t.Fatalf("newStore returned %v, want %v", err, errDatabaseNotOpen)
	}
}

func TestNewAuditPublisherReturnsNilWhenDisabled(t *testing.T) {
	observer, closers, err := newAuditPublisher(config.Config{}, nil)
	if err != nil {
		t.Fatalf("newAuditPublisher returned error: %v", err)
	}
	if observer != nil {
		t.Fatalf("observer = %#v, want nil interface", observer)
	}
	if len(closers) != 0 {
		t.Fatalf("closers = %d, want 0", len(closers))
	}
}

func TestNewAuditPublisherConfiguredObservers(t *testing.T) {
	tests := []struct {
		name       string
		configure  func(config.Config) config.Config
		wantCloser int
	}{
		{
			name: "file",
			configure: func(cfg config.Config) config.Config {
				cfg.AuditFile = filepath.Join(t.TempDir(), "audit.jsonl")
				return cfg
			},
			wantCloser: 2,
		},
		{
			name: "http",
			configure: func(cfg config.Config) config.Config {
				cfg.AuditURL = "http://audit.example/events"
				return cfg
			},
			wantCloser: 1,
		},
		{
			name: "both",
			configure: func(cfg config.Config) config.Config {
				cfg.AuditFile = filepath.Join(t.TempDir(), "audit.jsonl")
				cfg.AuditURL = "http://audit.example/events"
				return cfg
			},
			wantCloser: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observer, closers, err := newAuditPublisher(tt.configure(config.Config{}), nil)
			if err != nil {
				t.Fatalf("newAuditPublisher returned error: %v", err)
			}
			if observer == nil {
				t.Fatal("observer is nil")
			}
			if len(closers) != tt.wantCloser {
				t.Fatalf("closers = %d, want %d", len(closers), tt.wantCloser)
			}
			for _, closer := range closers {
				if err := closer.Close(); err != nil {
					t.Errorf("Close: %v", err)
				}
			}
		})
	}
}
