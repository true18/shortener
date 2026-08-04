package migrations

import (
	"database/sql"
	"embed"
	"errors"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed *.sql
var fs embed.FS

func Up(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}

	source, err := iofs.New(fs, ".")
	if err != nil {
		_ = db.Close()
		return err
	}

	driver, err := pgx.WithInstance(db, &pgx.Config{})
	if err != nil {
		_ = db.Close()
		return err
	}

	m, err := migrate.NewWithInstance("iofs", source, "pgx", driver)
	if err != nil {
		_ = driver.Close()
		return err
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	return nil
}
