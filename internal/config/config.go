package config

import (
	"flag"
	"os"
)

const (
	DefaultServerAddress = "localhost:8080"
	DefaultBaseURL       = "http://localhost:8080"
	EnvServerAddress     = "SERVER_ADDRESS"
	EnvBaseURL           = "BASE_URL"
)

type Config struct {
	ServerAddress string
	BaseURL       string
}

func Parse(args []string) (Config, error) {
	cfg := Config{
		ServerAddress: DefaultServerAddress,
		BaseURL:       DefaultBaseURL,
	}

	flags := flag.NewFlagSet("shortener", flag.ContinueOnError)
	flags.StringVar(&cfg.ServerAddress, "a", cfg.ServerAddress, "HTTP server address")
	flags.StringVar(&cfg.BaseURL, "b", cfg.BaseURL, "base URL for shortened links")

	if err := flags.Parse(args); err != nil {
		return Config{}, err
	}

	if value, ok := os.LookupEnv(EnvServerAddress); ok {
		cfg.ServerAddress = value
	}
	if value, ok := os.LookupEnv(EnvBaseURL); ok {
		cfg.BaseURL = value
	}

	return cfg, nil
}
