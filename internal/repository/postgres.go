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

func (p *Postgres) Save(originalURL string, userID string) (string, error) {
	id, exists, err := savePostgresURL(p.db, originalURL, userID)
	if err != nil {
		return "", err
	}
	if exists {
		return id, ErrURLExists
	}

	return id, nil
}

func (p *Postgres) SaveBatch(items []BatchItem, userID string) ([]BatchResult, error) {
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
		id, _, err := savePostgresURL(tx, item.OriginalURL, userID)
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

func savePostgresURL(db postgresSaver, originalURL string, userID string) (string, bool, error) {
	for {
		id, err := newID()
		if err != nil {
			return "", false, err
		}

		var savedID string
		err = db.QueryRow(`
			INSERT INTO urls (short_url, original_url, user_id)
			VALUES ($1, $2, $3)
			ON CONFLICT (original_url) DO NOTHING
			RETURNING short_url
		`, id, originalURL, userID).Scan(&savedID)
		if err == nil {
			return savedID, false, nil
		}
		if errors.Is(err, sql.ErrNoRows) {
			savedID, err := findShortURL(db, originalURL)
			if err != nil {
				return "", false, err
			}
			return savedID, true, nil
		}
		if isShortURLConflict(err) {
			continue
		}

		return "", false, err
	}
}

func findShortURL(db postgresSaver, originalURL string) (string, error) {
	var id string
	err := db.QueryRow(`
		SELECT short_url
		FROM urls
		WHERE original_url = $1
	`, originalURL).Scan(&id)
	if err != nil {
		return "", err
	}

	return id, nil
}

func (p *Postgres) Find(id string) (string, error) {
	var originalURL string
	var deleted bool
	err := p.db.QueryRow(`
		SELECT original_url, is_deleted
		FROM urls
		WHERE short_url = $1
	`, id).Scan(&originalURL, &deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if deleted {
		return "", ErrDeleted
	}

	return originalURL, nil
}

func (p *Postgres) FindByUserID(userID string) ([]UserURL, error) {
	rows, err := p.db.Query(`
		SELECT short_url, original_url
		FROM urls
		WHERE user_id = $1
		  AND is_deleted = FALSE
		ORDER BY id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	urls := make([]UserURL, 0)
	for rows.Next() {
		var item UserURL
		if err := rows.Scan(&item.ShortID, &item.OriginalURL); err != nil {
			return nil, err
		}

		urls = append(urls, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return urls, nil
}

func (p *Postgres) DeleteUserURLs(ids []string, userID string) error {
	if len(ids) == 0 {
		return nil
	}

	_, err := p.db.Exec(`
		UPDATE urls
		SET is_deleted = TRUE
		WHERE short_url = ANY($1::text[])
		  AND user_id = $2
	`, ids, userID)
	return err
}

func isShortURLConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == uniqueViolation &&
		pgErr.ConstraintName == "urls_short_url_key"
}
