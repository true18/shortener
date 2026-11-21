package server

import (
	"net/http"

	"github.com/true18/shortener/internal/config"
	"github.com/true18/shortener/internal/handler"
	"github.com/true18/shortener/internal/middleware"
	"github.com/true18/shortener/internal/repository"
	"go.uber.org/zap"
)

func New(cfg config.Config, logger *zap.Logger) (http.Handler, error) {
	store, err := repository.NewFile(cfg.FileStoragePath)
	if err != nil {
		return nil, err
	}

	h := handler.New(store, cfg.BaseURL)

	return middleware.RequestLogger(logger)(middleware.Gzip(h)), nil
}

func Run(cfg config.Config, logger *zap.Logger) error {
	h, err := New(cfg, logger)
	if err != nil {
		return err
	}

	return http.ListenAndServe(cfg.ServerAddress, h)
}
