package audit

import (
	"context"
	"encoding/json"
	"os"
	"sync"
)

// FileObserver appends audit events to a JSON Lines file.
type FileObserver struct {
	mu      sync.Mutex
	file    *os.File
	encoder *json.Encoder
}

// NewFileObserver opens an audit file for append, creating it when necessary.
func NewFileObserver(path string) (*FileObserver, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return &FileObserver{
		file:    file,
		encoder: json.NewEncoder(file),
	}, nil
}

// Notify appends one event as a single JSON line.
func (o *FileObserver) Notify(_ context.Context, event Event) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	return o.encoder.Encode(event)
}

// Close closes the audit file.
func (o *FileObserver) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	return o.file.Close()
}
