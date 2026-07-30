package repository

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNewFileMissingFileStartsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "storage.json")

	store, err := NewFile(path)
	if err != nil {
		t.Fatalf("NewFile returned error: %v", err)
	}

	if _, err := store.Find("unknown"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Find returned %v, want %v", err, ErrNotFound)
	}
}

func TestFileStorageSavesAndLoadsRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "storage.json")

	store, err := NewFile(path)
	if err != nil {
		t.Fatalf("NewFile returned error: %v", err)
	}

	firstID, err := store.Save("http://yandex.ru")
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	secondID, err := store.Save("http://ya.ru")
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}

	var records []fileRecord
	if err := json.Unmarshal(data, &records); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records len = %d, want %d", len(records), 2)
	}
	if records[0].UUID != "1" || records[0].ShortURL != firstID || records[0].OriginalURL != "http://yandex.ru" {
		t.Fatalf("first record = %+v", records[0])
	}
	if records[1].UUID != "2" || records[1].ShortURL != secondID || records[1].OriginalURL != "http://ya.ru" {
		t.Fatalf("second record = %+v", records[1])
	}

	loaded, err := NewFile(path)
	if err != nil {
		t.Fatalf("NewFile returned error: %v", err)
	}

	originalURL, err := loaded.Find(firstID)
	if err != nil {
		t.Fatalf("Find returned error: %v", err)
	}
	if originalURL != "http://yandex.ru" {
		t.Fatalf("originalURL = %q, want %q", originalURL, "http://yandex.ru")
	}

	sameID, err := loaded.Save("http://yandex.ru")
	if err != nil {
		t.Fatalf("Save existing URL returned error: %v", err)
	}
	if sameID != firstID {
		t.Fatalf("sameID = %q, want %q", sameID, firstID)
	}

	thirdID, err := loaded.Save("http://example.com")
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if thirdID == firstID || thirdID == secondID {
		t.Fatalf("thirdID = %q conflicts with previous ids", thirdID)
	}

	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if err := json.Unmarshal(data, &records); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("records len = %d, want %d", len(records), 3)
	}
	if records[2].UUID != "3" || records[2].ShortURL != thirdID || records[2].OriginalURL != "http://example.com" {
		t.Fatalf("third record = %+v", records[2])
	}
}

func TestNewFileBadJSONReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "storage.json")
	if err := os.WriteFile(path, []byte("{"), 0644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if _, err := NewFile(path); err == nil {
		t.Fatal("NewFile returned nil error")
	}
}
