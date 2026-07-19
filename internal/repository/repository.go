package repository

import "errors"

var ErrNotFound = errors.New("url not found")

type URLRepository interface {
	Save(originalURL string) (string, error)
	Find(id string) (string, error)
}
