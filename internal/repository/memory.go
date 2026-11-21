package repository

import (
	"crypto/rand"
	"math/big"
	"sync"
)

const (
	idChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	idLen   = 8
)

type Memory struct {
	mu   sync.RWMutex
	urls map[string]string
	ids  map[string]string
}

func NewMemory() *Memory {
	return &Memory{
		urls: make(map[string]string),
		ids:  make(map[string]string),
	}
}

func (m *Memory) Save(originalURL string) (string, error) {
	m.mu.RLock()
	id, ok := m.ids[originalURL]
	m.mu.RUnlock()
	if ok {
		return id, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if id, ok := m.ids[originalURL]; ok {
		return id, nil
	}

	for {
		id, err := newID()
		if err != nil {
			return "", err
		}
		if _, exists := m.urls[id]; exists {
			continue
		}

		m.urls[id] = originalURL
		m.ids[originalURL] = id
		return id, nil
	}
}

func (m *Memory) Find(id string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	originalURL, ok := m.urls[id]
	if !ok {
		return "", ErrNotFound
	}

	return originalURL, nil
}

func newID() (string, error) {
	id := make([]byte, idLen)
	max := big.NewInt(int64(len(idChars)))

	for i := range id {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		id[i] = idChars[n.Int64()]
	}

	return string(id), nil
}
