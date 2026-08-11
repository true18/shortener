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
	fileMu   sync.Mutex
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

func (m *Memory) Save(originalURL string, userID string) (string, error) {
	if m.filePath != "" {
		m.fileMu.Lock()
		defer m.fileMu.Unlock()
	}

	m.mu.Lock()
	if id, ok := m.ids[originalURL]; ok {
		m.mu.Unlock()
		return id, ErrURLExists
	}

	record, err := m.addURLLocked(originalURL, userID)
	if err != nil {
		m.mu.Unlock()
		return "", err
	}
	snapshot := cloneRecords(m.records)
	m.mu.Unlock()

	if err := m.save(snapshot); err != nil {
		m.rollback([]fileRecord{record})
		return "", err
	}

	return record.ShortURL, nil
}

func (m *Memory) SaveBatch(items []BatchItem, userID string) ([]BatchResult, error) {
	if len(items) == 0 {
		return []BatchResult{}, nil
	}

	if m.filePath != "" {
		m.fileMu.Lock()
		defer m.fileMu.Unlock()
	}

	m.mu.Lock()
	results, added, changed, err := m.saveBatchLocked(items, userID)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	snapshot := cloneRecords(m.records)
	m.mu.Unlock()

	if !changed {
		return results, nil
	}
	if err := m.save(snapshot); err != nil {
		m.rollback(added)
		return nil, err
	}

	return results, nil
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

func (m *Memory) FindByUserID(userID string) ([]UserURL, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	urls := make([]UserURL, 0)
	for _, record := range m.records {
		if record.UserID != userID {
			continue
		}

		urls = append(urls, UserURL{
			ShortID:     record.ShortURL,
			OriginalURL: record.OriginalURL,
		})
	}

	return urls, nil
}

type fileRecord struct {
	UUID        string `json:"uuid"`
	ShortURL    string `json:"short_url"`
	OriginalURL string `json:"original_url"`
	UserID      string `json:"user_id"`
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

	for _, record := range records {
		m.records = append(m.records, record)
		m.urls[record.ShortURL] = record.OriginalURL
		m.ids[record.OriginalURL] = record.ShortURL
	}

	m.nextUUID = nextUUID(m.records)

	return nil
}

func (m *Memory) save(records []fileRecord) error {
	if m.filePath == "" {
		return nil
	}

	dir := filepath.Dir(m.filePath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(m.filePath, data, 0644)
}

func (m *Memory) saveBatchLocked(items []BatchItem, userID string) ([]BatchResult, []fileRecord, bool, error) {
	results := make([]BatchResult, len(items))
	var added []fileRecord

	for i, item := range items {
		id, ok := m.ids[item.OriginalURL]
		if !ok {
			record, err := m.addURLLocked(item.OriginalURL, userID)
			if err != nil {
				return nil, nil, false, err
			}

			id = record.ShortURL
			added = append(added, record)
		}

		results[i] = BatchResult{
			CorrelationID: item.CorrelationID,
			ShortID:       id,
		}
	}

	return results, added, len(added) > 0, nil
}

func (m *Memory) addURLLocked(originalURL string, userID string) (fileRecord, error) {
	id, err := m.newIDLocked()
	if err != nil {
		return fileRecord{}, err
	}

	record := fileRecord{
		UUID:        strconv.Itoa(m.nextUUID),
		ShortURL:    id,
		OriginalURL: originalURL,
		UserID:      userID,
	}
	m.urls[id] = originalURL
	m.ids[originalURL] = id
	m.records = append(m.records, record)
	m.nextUUID++

	return record, nil
}

func (m *Memory) newIDLocked() (string, error) {
	for {
		id, err := newID()
		if err != nil {
			return "", err
		}
		if _, exists := m.urls[id]; !exists {
			return id, nil
		}
	}
}

func (m *Memory) rollback(records []fileRecord) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, record := range records {
		if m.ids[record.OriginalURL] == record.ShortURL {
			delete(m.ids, record.OriginalURL)
		}
		if m.urls[record.ShortURL] == record.OriginalURL {
			delete(m.urls, record.ShortURL)
		}

		for i, saved := range m.records {
			if saved.UUID == record.UUID && saved.ShortURL == record.ShortURL {
				m.records = append(m.records[:i], m.records[i+1:]...)
				break
			}
		}
	}

	m.nextUUID = nextUUID(m.records)
}

func cloneRecords(records []fileRecord) []fileRecord {
	return append([]fileRecord(nil), records...)
}

func nextUUID(records []fileRecord) int {
	maxUUID := 0
	for _, record := range records {
		uuid, err := strconv.Atoi(record.UUID)
		if err == nil && uuid > maxUUID {
			maxUUID = uuid
		}
	}

	if len(records) > maxUUID {
		maxUUID = len(records)
	}

	return maxUUID + 1
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
