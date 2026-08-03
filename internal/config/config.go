package config

import (
	"flag"
	"os"
)

const (
	DefaultServerAddress   = "localhost:8080"
	DefaultBaseURL         = "http://localhost:8080"
	DefaultFileStoragePath = "storage.json"
	EnvServerAddress       = "SERVER_ADDRESS"
	EnvBaseURL             = "BASE_URL"
	EnvFileStoragePath     = "FILE_STORAGE_PATH"
)

type Config struct {
	ServerAddress   string
	BaseURL         string
	FileStoragePath string
}

func Parse(args []string) (Config, error) {
	cfg := Config{
		ServerAddress:   DefaultServerAddress,
		BaseURL:         DefaultBaseURL,
		FileStoragePath: DefaultFileStoragePath,
	}

	flags := flag.NewFlagSet("shortener", flag.ContinueOnError)
	flags.StringVar(&cfg.ServerAddress, "a", cfg.ServerAddress, "HTTP server address")
	flags.StringVar(&cfg.BaseURL, "b", cfg.BaseURL, "base URL for shortened links")
	flags.StringVar(&cfg.FileStoragePath, "f", cfg.FileStoragePath, "file storage path")

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

	return cfg, nil
}
