package repository

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	idChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	idLen   = 8
)

type Memory struct {
	mu       sync.RWMutex
	urls     map[string]string
	ids      map[string]string
	records  []fileRecord
	nextUUID int
	filePath string
}

func NewMemory() *Memory {
	return &Memory{
		urls:     make(map[string]string),
		ids:      make(map[string]string),
		nextUUID: 1,
	}
}

func NewFile(path string) (*Memory, error) {
	store := NewMemory()
	store.filePath = path

	if err := store.load(); err != nil {
		return nil, err
	}

	return store, nil
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

		record := fileRecord{
			UUID:        strconv.Itoa(m.nextUUID),
			ShortURL:    id,
			OriginalURL: originalURL,
		}
		m.urls[id] = originalURL
		m.ids[originalURL] = id
		m.records = append(m.records, record)
		m.nextUUID++

		if err := m.save(); err != nil {
			delete(m.urls, id)
			delete(m.ids, originalURL)
			m.records = m.records[:len(m.records)-1]
			m.nextUUID--
			return "", err
		}

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

type fileRecord struct {
	UUID        string `json:"uuid"`
	ShortURL    string `json:"short_url"`
	OriginalURL string `json:"original_url"`
}

func (m *Memory) load() error {
	if m.filePath == "" {
		return nil
	}

	data, err := os.ReadFile(m.filePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil
	}

	var records []fileRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return err
	}

	maxUUID := 0
	for _, record := range records {
		m.records = append(m.records, record)
		m.urls[record.ShortURL] = record.OriginalURL
		m.ids[record.OriginalURL] = record.ShortURL

		uuid, err := strconv.Atoi(record.UUID)
		if err == nil && uuid > maxUUID {
			maxUUID = uuid
		}
	}

	if len(records) > maxUUID {
		maxUUID = len(records)
	}
	m.nextUUID = maxUUID + 1

	return nil
}

func (m *Memory) save() error {
	if m.filePath == "" {
		return nil
	}

	dir := filepath.Dir(m.filePath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	data, err := json.MarshalIndent(m.records, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(m.filePath, data, 0644)
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
