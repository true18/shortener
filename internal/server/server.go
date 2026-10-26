package server

import (
	"database/sql"
	"errors"
	"io"
	"net/http"

	"github.com/true18/shortener/internal/audit"
	"github.com/true18/shortener/internal/auth"
	"github.com/true18/shortener/internal/config"
	"github.com/true18/shortener/internal/handler"
	"github.com/true18/shortener/internal/middleware"
	"github.com/true18/shortener/internal/migrations"
	"github.com/true18/shortener/internal/repository"
	"github.com/true18/shortener/internal/service"
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
	if logger == nil {
		logger = zap.NewNop()
	}

	store, err := newStore(cfg, db)
	if err != nil {
		return nil, err
	}
	publisher, closers, err := newAuditPublisher(cfg)
	if err != nil {
		return nil, err
	}

	var pinger handler.Pinger
	if db != nil {
		pinger = db
	}

	deletes := service.NewDeleteQueue(store)
	h := handler.NewWithAudit(store, cfg.BaseURL, pinger, deletes, publisher, logger)
	wrapped := middleware.RequestLogger(logger)(auth.Middleware(middleware.Gzip(h)))

	return &managedHandler{Handler: wrapped, closers: closers}, nil
}

func Run(cfg config.Config, logger *zap.Logger, db *sql.DB) error {
	h, err := New(cfg, logger, db)
	if err != nil {
		return err
	}
	if closer, ok := h.(io.Closer); ok {
		defer func() {
			if err := closer.Close(); err != nil {
				logger.Error("close server resources", zap.Error(err))
			}
		}()
	}

	return http.ListenAndServe(cfg.ServerAddress, h)
}

type managedHandler struct {
	http.Handler
	closers []io.Closer
}

func (h *managedHandler) Close() error {
	var errs []error
	for _, closer := range h.closers {
		if err := closer.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func newAuditPublisher(cfg config.Config) (*audit.Publisher, []io.Closer, error) {
	if cfg.AuditFile == "" && cfg.AuditURL == "" {
		return nil, nil, nil
	}

	observers := make([]audit.Observer, 0, 2)
	closers := make([]io.Closer, 0, 1)
	if cfg.AuditFile != "" {
		observer, err := audit.NewFileObserver(cfg.AuditFile)
		if err != nil {
			return nil, nil, err
		}
		observers = append(observers, observer)
		closers = append(closers, observer)
	}
	if cfg.AuditURL != "" {
		observer, err := audit.NewHTTPObserver(cfg.AuditURL, nil)
		if err != nil {
			for _, closer := range closers {
				_ = closer.Close()
			}
			return nil, nil, err
		}
		observers = append(observers, observer)
	}

	return audit.NewPublisher(observers...), closers, nil
}

func newStore(cfg config.Config, db *sql.DB) (repository.Store, error) {
	switch storageFor(cfg) {
	case storagePostgres:
		if db == nil {
			return nil, errDatabaseNotOpen
		}
		if err := migrations.Up(db); err != nil {
			return nil, err
		}

		return repository.NewPostgres(db), nil
	case storageFile:
		store, err := repository.NewFile(cfg.FileStoragePath)
		if err != nil {
			return nil, err
		}

		return store, nil
	default:
		return repository.NewMemory(), nil
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
