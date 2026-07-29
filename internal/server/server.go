package server

import (
	"net/http"

	"github.com/true18/shortener/internal/config"
	"github.com/true18/shortener/internal/handler"
	"github.com/true18/shortener/internal/middleware"
	"github.com/true18/shortener/internal/repository"
	"go.uber.org/zap"
)

func New(cfg config.Config, logger *zap.Logger) http.Handler {
	h := handler.New(repository.NewMemory(), cfg.BaseURL)

	return middleware.RequestLogger(logger)(h)
}

func Run(cfg config.Config, logger *zap.Logger) error {
	return http.ListenAndServe(cfg.ServerAddress, New(cfg, logger))
}
