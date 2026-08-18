package repository

import "errors"

var (
	ErrNotFound  = errors.New("url not found")
	ErrURLExists = errors.New("url already exists")
	ErrDeleted   = errors.New("url deleted")
)

type Store interface {
	Save(originalURL string, userID string) (string, error)
	SaveBatch(items []BatchItem, userID string) ([]BatchResult, error)
	Find(id string) (string, error)
	FindByUserID(userID string) ([]UserURL, error)
	DeleteUserURLs(ids []string, userID string) error
}

type BatchItem struct {
	CorrelationID string
	OriginalURL   string
}

type BatchResult struct {
	CorrelationID string
	ShortID       string
}

type UserURL struct {
	ShortID     string
	OriginalURL string
}
