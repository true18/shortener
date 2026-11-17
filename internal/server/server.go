// Package server assembles the URL service's storage, handlers, and middleware.
package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/true18/shortener/internal/audit"
	"github.com/true18/shortener/internal/auth"
	"github.com/true18/shortener/internal/config"
	"github.com/true18/shortener/internal/grpcserver"
	"github.com/true18/shortener/internal/handler"
	"github.com/true18/shortener/internal/middleware"
	"github.com/true18/shortener/internal/migrations"
	"github.com/true18/shortener/internal/repository"
	"github.com/true18/shortener/internal/service"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type storageKind string

const (
	storageMemory   storageKind = "memory"
	storageFile     storageKind = "file"
	storagePostgres storageKind = "postgres"
)

var errDatabaseNotOpen = errors.New("database is not open")

const shutdownTimeout = 30 * time.Second

// New builds the HTTP handler, initializes configured storage and audit observers, and owns their closers.
func New(cfg config.Config, logger *zap.Logger, db *sql.DB) (http.Handler, error) {
	managed, _, err := newManagedHandler(cfg, logger, db)
	if err != nil {
		return nil, err
	}
	return managed, nil
}

func newManagedHandler(cfg config.Config, logger *zap.Logger, db *sql.DB) (*managedHandler, *service.Shortener, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	store, err := newStore(cfg, db)
	if err != nil {
		return nil, nil, err
	}
	auditor, closers, err := newAuditPublisher(cfg, logger)
	if err != nil {
		return nil, nil, err
	}

	var pinger handler.Pinger
	if db != nil {
		pinger = db
	}

	deletes := service.NewDeleteQueue(store)
	trustedSubnet, err := parseTrustedSubnet(cfg.TrustedSubnet)
	if err != nil {
		deletes.Close()
		for _, closer := range closers {
			_ = closer.Close()
		}
		return nil, nil, err
	}
	app := service.NewShortener(store, cfg.BaseURL, auditor, logger)
	h := handler.NewWithApplication(store, pinger, deletes, app, trustedSubnet)
	wrapped := middleware.RequestLogger(logger)(auth.Middleware(middleware.Gzip(h)))

	return &managedHandler{Handler: wrapped, deletes: deletes, closers: closers}, app, nil
}

func parseTrustedSubnet(value string) (*net.IPNet, error) {
	if value == "" {
		return nil, nil
	}

	_, network, err := net.ParseCIDR(value)
	if err != nil {
		return nil, fmt.Errorf("parse trusted subnet: %w", err)
	}

	return network, nil
}

// Run serves requests until the server stops and closes server-owned resources on exit.
func Run(cfg config.Config, logger *zap.Logger, db *sql.DB) error {
	return RunContext(context.Background(), cfg, logger, db)
}

// RunContext serves HTTP or HTTPS requests and gracefully stops when ctx is done.
func RunContext(ctx context.Context, cfg config.Config, logger *zap.Logger, db *sql.DB) (runErr error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	h, app, err := newManagedHandler(cfg, logger, db)
	if err != nil {
		return err
	}
	defer func() {
		if err := h.Close(); err != nil {
			logger.Error("close server resources", zap.Error(err))
			runErr = errors.Join(runErr, fmt.Errorf("close server resources: %w", err))
		}
	}()

	server, err := newHTTPServer(cfg, h)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", cfg.ServerAddress)
	if err != nil {
		return err
	}
	defer listener.Close()

	if cfg.GRPCAddress == "" {
		runErr = runHTTPServer(ctx, server, listener, cfg.EnableHTTPS, shutdownTimeout)
	} else {
		grpcListener, listenErr := net.Listen("tcp", cfg.GRPCAddress)
		if listenErr != nil {
			return listenErr
		}
		defer grpcListener.Close()

		var grpcOptions []grpc.ServerOption
		if cfg.EnableHTTPS {
			grpcOptions = append(grpcOptions, grpc.Creds(credentials.NewTLS(server.TLSConfig.Clone())))
		}
		grpcServer := grpcserver.New(app, grpcOptions...)
		runErr = runHTTPAndGRPC(ctx, server, listener, cfg.EnableHTTPS, grpcServer, grpcListener, shutdownTimeout)
	}
	if runErr != nil {
		logger.Error("serve requests", zap.Error(runErr))
	}
	return runErr
}

func runHTTPAndGRPC(
	ctx context.Context,
	httpServer *http.Server,
	httpListener net.Listener,
	enableHTTPS bool,
	grpcServer *grpc.Server,
	grpcListener net.Listener,
	timeout time.Duration,
) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	httpDone := make(chan error, 1)
	go func() {
		httpDone <- runHTTPServer(runCtx, httpServer, httpListener, enableHTTPS, timeout)
	}()
	grpcDone := make(chan error, 1)
	go func() {
		grpcDone <- grpcServer.Serve(grpcListener)
	}()

	select {
	case httpErr := <-httpDone:
		grpcStopErr := stopGRPCServer(grpcServer, timeout)
		return errors.Join(httpErr, grpcStopErr, normalizeGRPCError(<-grpcDone))
	case grpcErr := <-grpcDone:
		cancel()
		return errors.Join(normalizeGRPCError(grpcErr), <-httpDone)
	case <-ctx.Done():
		cancel()
		grpcStopErr := stopGRPCServer(grpcServer, timeout)
		return errors.Join(<-httpDone, grpcStopErr, normalizeGRPCError(<-grpcDone))
	}
}

func stopGRPCServer(server *grpc.Server, timeout time.Duration) error {
	done := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(done)
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return nil
	case <-timer.C:
		server.Stop()
		return fmt.Errorf("shutdown gRPC server: %w", context.DeadlineExceeded)
	}
}

func normalizeGRPCError(err error) error {
	if errors.Is(err, grpc.ErrServerStopped) {
		return nil
	}
	return err
}

func runHTTPServer(ctx context.Context, server *http.Server, listener net.Listener, enableHTTPS bool, timeout time.Duration) error {
	serveErrors := make(chan error, 1)
	go func() {
		if enableHTTPS {
			serveErrors <- server.ServeTLS(listener, "", "")
			return
		}
		serveErrors <- server.Serve(listener)
	}()

	select {
	case err := <-serveErrors:
		return normalizeServerError(err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			closeErr := server.Close()
			serveErr := normalizeServerError(<-serveErrors)
			return errors.Join(
				fmt.Errorf("shutdown HTTP server: %w", err),
				closeErr,
				serveErr,
			)
		}

		return normalizeServerError(<-serveErrors)
	}
}

func normalizeServerError(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

type managedHandler struct {
	http.Handler
	closers   []io.Closer
	deletes   *service.DeleteQueue
	closeOnce sync.Once
	closeErr  error
}

func (h *managedHandler) Close() error {
	h.closeOnce.Do(func() {
		if h.deletes != nil {
			h.deletes.Close()
		}

		var errs []error
		for _, closer := range h.closers {
			if err := closer.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		h.closeErr = errors.Join(errs...)
	})

	return h.closeErr
}

func newAuditPublisher(cfg config.Config, logger *zap.Logger) (audit.Observer, []io.Closer, error) {
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

	queue := audit.NewQueue(audit.NewPublisher(observers...), logger)
	return queue, append([]io.Closer{queue}, closers...), nil
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
