package server

import (
	"errors"
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
