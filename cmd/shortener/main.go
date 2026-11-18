// Package main starts the URL shortener HTTP service.
package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/true18/shortener/internal/config"
	"github.com/true18/shortener/internal/server"
	"go.uber.org/zap"
)

func main() {
	finish(run())
}

func finish(err error) {
	if err == nil {
		return
	}

	fmt.Fprintln(os.Stderr, err)
	exit(1)
}

func exit(code int) {
	os.Exit(code)
}

func run() error {
	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		return fmt.Errorf("config error: %w", err)
	}

	logger, err := zap.NewProduction()
	if err != nil {
		return fmt.Errorf("logger error: %w", err)
	}
	defer func() {
		_ = logger.Sync()
	}()

	db, err := openDB(cfg.DatabaseDSN)
	if err != nil {
		return fmt.Errorf("database error: %w", err)
	}
	if db != nil {
		defer db.Close()
	}

	if err := server.Run(cfg, logger, db); err != nil {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

func openDB(dsn string) (*sql.DB, error) {
	if dsn == "" {
		return nil, nil
	}

	return sql.Open("pgx", dsn)
}
