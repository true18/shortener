// Package repository provides in-memory, file-backed, and PostgreSQL URL stores.
package repository

import "errors"

var (
	// ErrNotFound indicates that a short identifier has no stored URL.
	ErrNotFound = errors.New("url not found")
	// ErrURLExists indicates that Save found an existing original URL. The existing short ID is returned.
	ErrURLExists = errors.New("url already exists")
	// ErrDeleted indicates that the short URL exists but has been soft-deleted.
	ErrDeleted = errors.New("url deleted")
)

// Store defines the URL operations shared by all repository implementations.
type Store interface {
	// Save stores an original URL and returns its short ID. A duplicate returns the existing ID with ErrURLExists.
	Save(originalURL string, userID string) (string, error)
	// SaveBatch stores URLs for one user and preserves the input order in its results.
	SaveBatch(items []BatchItem, userID string) ([]BatchResult, error)
	// Find returns the original URL for an active short ID, or a repository error.
	Find(id string) (string, error)
	// FindByUserID returns active URLs owned by userID.
	FindByUserID(userID string) ([]UserURL, error)
	// DeleteUserURLs marks the listed URLs as deleted when they belong to userID.
	DeleteUserURLs(ids []string, userID string) error
}

// BatchItem is one original URL and its caller-provided correlation ID.
type BatchItem struct {
	// CorrelationID identifies this item in the caller's batch.
	CorrelationID string
	// OriginalURL is the URL to shorten.
	OriginalURL string
}

// BatchResult pairs a saved short ID with its input correlation ID.
type BatchResult struct {
	// CorrelationID is copied from the corresponding input item.
	CorrelationID string
	// ShortID is the generated or previously stored identifier.
	ShortID string
}

// UserURL describes an active URL owned by a user.
type UserURL struct {
	// ShortID identifies the active shortened URL.
	ShortID string
	// OriginalURL is the destination associated with ShortID.
	OriginalURL string
}
