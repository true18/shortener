package repository

import "errors"

var ErrNotFound = errors.New("url not found")

type Store interface {
	Save(originalURL string) (string, error)
	SaveBatch(items []BatchItem) ([]BatchResult, error)
	Find(id string) (string, error)
}

type BatchItem struct {
	CorrelationID string
	OriginalURL   string
}

type BatchResult struct {
	CorrelationID string
	ShortID       string
}
