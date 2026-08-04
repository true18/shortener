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

func TestMemorySaveBatch(t *testing.T) {
	store := NewMemory()

	results, err := store.SaveBatch([]BatchItem{
		{CorrelationID: "1", OriginalURL: "http://yandex.ru"},
		{CorrelationID: "2", OriginalURL: "http://ya.ru"},
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

	originalURL, err := store.Find(results[0].ShortID)
	if err != nil {
		t.Fatalf("Find returned error: %v", err)
	}
	if originalURL != "http://yandex.ru" {
		t.Fatalf("originalURL = %q, want %q", originalURL, "http://yandex.ru")
	}

	again, err := store.SaveBatch([]BatchItem{
		{CorrelationID: "3", OriginalURL: "http://yandex.ru"},
	})
	if err != nil {
		t.Fatalf("SaveBatch existing URL returned error: %v", err)
	}
	if again[0].ShortID != results[0].ShortID {
		t.Fatalf("short id = %q, want %q", again[0].ShortID, results[0].ShortID)
	}

	id, err := store.Save("http://example.com")
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if id == "" {
		t.Fatal("Save returned empty id")
	}
}

func TestFileStorageSaveBatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "storage.json")

	store, err := NewFile(path)
	if err != nil {
		t.Fatalf("NewFile returned error: %v", err)
	}

	results, err := store.SaveBatch([]BatchItem{
		{CorrelationID: "1", OriginalURL: "http://yandex.ru"},
		{CorrelationID: "2", OriginalURL: "http://ya.ru"},
	})
	if err != nil {
		t.Fatalf("SaveBatch returned error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results len = %d, want %d", len(results), 2)
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
	if records[0].UUID != "1" || records[0].ShortURL != results[0].ShortID || records[0].OriginalURL != "http://yandex.ru" {
		t.Fatalf("first record = %+v", records[0])
	}
	if records[1].UUID != "2" || records[1].ShortURL != results[1].ShortID || records[1].OriginalURL != "http://ya.ru" {
		t.Fatalf("second record = %+v", records[1])
	}

	loaded, err := NewFile(path)
	if err != nil {
		t.Fatalf("NewFile returned error: %v", err)
	}

	originalURL, err := loaded.Find(results[1].ShortID)
	if err != nil {
		t.Fatalf("Find returned error: %v", err)
	}
	if originalURL != "http://ya.ru" {
		t.Fatalf("originalURL = %q, want %q", originalURL, "http://ya.ru")
	}

	again, err := loaded.SaveBatch([]BatchItem{
		{CorrelationID: "3", OriginalURL: "http://yandex.ru"},
	})
	if err != nil {
		t.Fatalf("SaveBatch existing URL returned error: %v", err)
	}
	if again[0].ShortID != results[0].ShortID {
		t.Fatalf("short id = %q, want %q", again[0].ShortID, results[0].ShortID)
	}

	id, err := loaded.Save("http://example.com")
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if id == "" {
		t.Fatal("Save returned empty id")
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
