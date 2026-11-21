package server

import (
	"net/http"

	"github.com/true18/shortener/internal/config"
	"github.com/true18/shortener/internal/handler"
	"github.com/true18/shortener/internal/repository"
)

func New(cfg config.Config) http.Handler {
	return handler.New(repository.NewMemory(), cfg.BaseURL)
}

func Run(cfg config.Config) error {
	return http.ListenAndServe(cfg.ServerAddress, New(cfg))
}
