package repository

import (
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

const uniqueViolation = "23505"

type Postgres struct {
	db *sql.DB
}

func NewPostgres(db *sql.DB) *Postgres {
	return &Postgres{db: db}
}

func (p *Postgres) Save(originalURL string) (string, error) {
	return savePostgresURL(p.db, originalURL)
}

func (p *Postgres) SaveBatch(items []BatchItem) ([]BatchResult, error) {
	if len(items) == 0 {
		return []BatchResult{}, nil
	}

	tx, err := p.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	results := make([]BatchResult, len(items))
	for i, item := range items {
		id, err := savePostgresURL(tx, item.OriginalURL)
		if err != nil {
			return nil, err
		}

		results[i] = BatchResult{
			CorrelationID: item.CorrelationID,
			ShortID:       id,
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return results, nil
}

type postgresSaver interface {
	QueryRow(query string, args ...any) *sql.Row
}

func savePostgresURL(db postgresSaver, originalURL string) (string, error) {
	for {
		id, err := newID()
		if err != nil {
			return "", err
		}

		var savedID string
		err = db.QueryRow(`
			INSERT INTO urls (short_url, original_url)
			VALUES ($1, $2)
			ON CONFLICT (original_url) DO UPDATE SET original_url = EXCLUDED.original_url
			RETURNING short_url
		`, id, originalURL).Scan(&savedID)
		if err == nil {
			return savedID, nil
		}
		if isShortURLConflict(err) {
			continue
		}

		return "", err
	}
}

func (p *Postgres) Find(id string) (string, error) {
	var originalURL string
	err := p.db.QueryRow(`
		SELECT original_url
		FROM urls
		WHERE short_url = $1
	`, id).Scan(&originalURL)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}

	return originalURL, nil
}

func isShortURLConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == uniqueViolation &&
		pgErr.ConstraintName == "urls_short_url_key"
}
