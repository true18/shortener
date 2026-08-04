package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/true18/shortener/internal/migrations"
)

func TestIsShortURLConflict(t *testing.T) {
	err := &pgconn.PgError{
		Code:           uniqueViolation,
		ConstraintName: "urls_short_url_key",
	}

	if !isShortURLConflict(err) {
		t.Fatal("isShortURLConflict returned false")
	}
}

func TestIsShortURLConflictForOtherConstraint(t *testing.T) {
	err := &pgconn.PgError{
		Code:           uniqueViolation,
		ConstraintName: "urls_original_url_key",
	}

	if isShortURLConflict(err) {
		t.Fatal("isShortURLConflict returned true")
	}
}

func TestPostgresStoreIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()

	if err := migrations.Up(db); err != nil {
		t.Fatalf("migrations.Up returned error: %v", err)
	}

	store := NewPostgres(db)
	originalURL := fmt.Sprintf("https://example.com/%d", time.Now().UnixNano())

	id, err := store.Save(originalURL)
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if id == "" {
		t.Fatal("Save returned empty id")
	}

	sameID, err := store.Save(originalURL)
	if err != nil {
		t.Fatalf("Save existing URL returned error: %v", err)
	}
	if sameID != id {
		t.Fatalf("sameID = %q, want %q", sameID, id)
	}

	got, err := store.Find(id)
	if err != nil {
		t.Fatalf("Find returned error: %v", err)
	}
	if got != originalURL {
		t.Fatalf("Find returned %q, want %q", got, originalURL)
	}

	if _, err := store.Find("unknown"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Find unknown returned %v, want %v", err, ErrNotFound)
	}

	batchURL := fmt.Sprintf("https://batch.example.com/%d", time.Now().UnixNano())
	results, err := store.SaveBatch([]BatchItem{
		{CorrelationID: "1", OriginalURL: batchURL},
		{CorrelationID: "2", OriginalURL: originalURL},
	})
	if err != nil {
		t.Fatalf("SaveBatch returned error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results len = %d, want %d", len(results), 2)
	}
	if results[0].CorrelationID != "1" || results[1].CorrelationID != "2" {
		t.Fatalf("correlation ids = %q, %q", results[0].CorrelationID, results[1].CorrelationID)
	}
	if results[1].ShortID != id {
		t.Fatalf("existing short id = %q, want %q", results[1].ShortID, id)
	}

	got, err = store.Find(results[0].ShortID)
	if err != nil {
		t.Fatalf("Find batch URL returned error: %v", err)
	}
	if got != batchURL {
		t.Fatalf("Find returned %q, want %q", got, batchURL)
	}
}
