package config

import (
	"flag"
	"os"
)

const (
	DefaultServerAddress   = "localhost:8080"
	DefaultBaseURL         = "http://localhost:8080"
	DefaultFileStoragePath = "storage.json"
	DefaultDatabaseDSN     = ""
	EnvServerAddress       = "SERVER_ADDRESS"
	EnvBaseURL             = "BASE_URL"
	EnvFileStoragePath     = "FILE_STORAGE_PATH"
	EnvDatabaseDSN         = "DATABASE_DSN"
)

type Config struct {
	ServerAddress   string
	BaseURL         string
	FileStoragePath string
	DatabaseDSN     string
}

func Parse(args []string) (Config, error) {
	cfg := Config{
		ServerAddress:   DefaultServerAddress,
		BaseURL:         DefaultBaseURL,
		FileStoragePath: DefaultFileStoragePath,
		DatabaseDSN:     DefaultDatabaseDSN,
	}

	flags := flag.NewFlagSet("shortener", flag.ContinueOnError)
	flags.StringVar(&cfg.ServerAddress, "a", cfg.ServerAddress, "HTTP server address")
	flags.StringVar(&cfg.BaseURL, "b", cfg.BaseURL, "base URL for shortened links")
	flags.StringVar(&cfg.FileStoragePath, "f", cfg.FileStoragePath, "file storage path")
	flags.StringVar(&cfg.DatabaseDSN, "d", cfg.DatabaseDSN, "database DSN")

	if err := flags.Parse(args); err != nil {
		return Config{}, err
	}

	if value, ok := os.LookupEnv(EnvServerAddress); ok {
		cfg.ServerAddress = value
	}
	if value, ok := os.LookupEnv(EnvBaseURL); ok {
		cfg.BaseURL = value
	}
	if value, ok := os.LookupEnv(EnvFileStoragePath); ok {
		cfg.FileStoragePath = value
	}
	if value, ok := os.LookupEnv(EnvDatabaseDSN); ok {
		cfg.DatabaseDSN = value
	}

	return cfg, nil
}
