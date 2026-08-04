package server

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/true18/shortener/internal/config"
	"github.com/true18/shortener/internal/handler"
	"github.com/true18/shortener/internal/middleware"
	"github.com/true18/shortener/internal/migrations"
	"github.com/true18/shortener/internal/repository"
	"go.uber.org/zap"
)

type storageKind string

const (
	storageMemory   storageKind = "memory"
	storageFile     storageKind = "file"
	storagePostgres storageKind = "postgres"
)

var errDatabaseNotOpen = errors.New("database is not open")

func New(cfg config.Config, logger *zap.Logger, db *sql.DB) (http.Handler, error) {
	store, pinger, err := newStore(cfg, db)
	if err != nil {
		return nil, err
	}

	h := handler.New(store, cfg.BaseURL, pinger)

	return middleware.RequestLogger(logger)(middleware.Gzip(h)), nil
}

func Run(cfg config.Config, logger *zap.Logger, db *sql.DB) error {
	h, err := New(cfg, logger, db)
	if err != nil {
		return err
	}

	return http.ListenAndServe(cfg.ServerAddress, h)
}

func newStore(cfg config.Config, db *sql.DB) (repository.Store, handler.Pinger, error) {
	switch storageFor(cfg) {
	case storagePostgres:
		if db == nil {
			return nil, nil, errDatabaseNotOpen
		}
		if err := migrations.Up(db); err != nil {
			return nil, nil, err
		}

		return repository.NewPostgres(db), db, nil
	case storageFile:
		store, err := repository.NewFile(cfg.FileStoragePath)
		if err != nil {
			return nil, nil, err
		}

		return store, nil, nil
	default:
		return repository.NewMemory(), nil, nil
	}
}

func storageFor(cfg config.Config) storageKind {
	if cfg.DatabaseDSN != "" {
		return storagePostgres
	}
	if cfg.FileStoragePathSet && cfg.FileStoragePath != "" {
		return storageFile
	}

	return storageMemory
}
