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
	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger error: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = logger.Sync()
	}()

	db, err := openDB(cfg.DatabaseDSN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "database error: %v\n", err)
		os.Exit(1)
	}
	if db != nil {
		defer db.Close()
	}

	if err := server.Run(cfg, logger, db); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func openDB(dsn string) (*sql.DB, error) {
	if dsn == "" {
		return nil, nil
	}

	return sql.Open("pgx", dsn)
}
