package repository

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const (
	testUserID  = "user-1"
	otherUserID = "user-2"
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

	firstID, err := store.Save("http://yandex.ru", testUserID)
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	secondID, err := store.Save("http://ya.ru", otherUserID)
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
	if records[0].UUID != "1" || records[0].ShortURL != firstID || records[0].OriginalURL != "http://yandex.ru" || records[0].UserID != testUserID {
		t.Fatalf("first record = %+v", records[0])
	}
	if records[1].UUID != "2" || records[1].ShortURL != secondID || records[1].OriginalURL != "http://ya.ru" || records[1].UserID != otherUserID {
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

	userURLs, err := loaded.FindByUserID(testUserID)
	if err != nil {
		t.Fatalf("FindByUserID returned error: %v", err)
	}
	if len(userURLs) != 1 || userURLs[0].ShortID != firstID || userURLs[0].OriginalURL != "http://yandex.ru" {
		t.Fatalf("user urls = %+v", userURLs)
	}

	sameID, err := loaded.Save("http://yandex.ru", otherUserID)
	if !errors.Is(err, ErrURLExists) {
		t.Fatalf("Save existing URL returned %v, want %v", err, ErrURLExists)
	}
	if sameID != firstID {
		t.Fatalf("sameID = %q, want %q", sameID, firstID)
	}

	thirdID, err := loaded.Save("http://example.com", testUserID)
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
	if records[2].UUID != "3" || records[2].ShortURL != thirdID || records[2].OriginalURL != "http://example.com" || records[2].UserID != testUserID {
		t.Fatalf("third record = %+v", records[2])
	}
}

func TestMemorySaveDuplicate(t *testing.T) {
	store := NewMemory()

	id, err := store.Save("http://yandex.ru", testUserID)
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	sameID, err := store.Save("http://yandex.ru", otherUserID)
	if !errors.Is(err, ErrURLExists) {
		t.Fatalf("Save existing URL returned %v, want %v", err, ErrURLExists)
	}
	if sameID != id {
		t.Fatalf("sameID = %q, want %q", sameID, id)
	}

	otherURLs, err := store.FindByUserID(otherUserID)
	if err != nil {
		t.Fatalf("FindByUserID returned error: %v", err)
	}
	if len(otherURLs) != 0 {
		t.Fatalf("other user urls = %+v, want empty", otherURLs)
	}
}

func TestMemoryFindByUserID(t *testing.T) {
	store := NewMemory()

	firstID, err := store.Save("http://yandex.ru", testUserID)
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	secondID, err := store.Save("http://ya.ru", testUserID)
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	_, err = store.Save("http://example.com", otherUserID)
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	urls, err := store.FindByUserID(testUserID)
	if err != nil {
		t.Fatalf("FindByUserID returned error: %v", err)
	}
	if len(urls) != 2 {
		t.Fatalf("urls len = %d, want %d", len(urls), 2)
	}
	if urls[0].ShortID != firstID || urls[0].OriginalURL != "http://yandex.ru" {
		t.Fatalf("first url = %+v", urls[0])
	}
	if urls[1].ShortID != secondID || urls[1].OriginalURL != "http://ya.ru" {
		t.Fatalf("second url = %+v", urls[1])
	}
}

func TestMemoryDeleteUserURLs(t *testing.T) {
	store := NewMemory()

	firstID, err := store.Save("http://yandex.ru", testUserID)
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	secondID, err := store.Save("http://ya.ru", testUserID)
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	otherID, err := store.Save("http://example.com", otherUserID)
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	if err := store.DeleteUserURLs([]string{firstID, otherID, "unknown"}, testUserID); err != nil {
		t.Fatalf("DeleteUserURLs returned error: %v", err)
	}

	if _, err := store.Find(firstID); !errors.Is(err, ErrDeleted) {
		t.Fatalf("Find deleted returned %v, want %v", err, ErrDeleted)
	}
	if got, err := store.Find(secondID); err != nil || got != "http://ya.ru" {
		t.Fatalf("Find second = %q, %v", got, err)
	}
	if got, err := store.Find(otherID); err != nil || got != "http://example.com" {
		t.Fatalf("Find other = %q, %v", got, err)
	}

	urls, err := store.FindByUserID(testUserID)
	if err != nil {
		t.Fatalf("FindByUserID returned error: %v", err)
	}
	if len(urls) != 1 || urls[0].ShortID != secondID {
		t.Fatalf("user urls = %+v", urls)
	}
}

func TestMemorySaveBatch(t *testing.T) {
	store := NewMemory()

	results, err := store.SaveBatch([]BatchItem{
		{CorrelationID: "1", OriginalURL: "http://yandex.ru"},
		{CorrelationID: "2", OriginalURL: "http://ya.ru"},
	}, testUserID)
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
	}, otherUserID)
	if err != nil {
		t.Fatalf("SaveBatch existing URL returned error: %v", err)
	}
	if again[0].ShortID != results[0].ShortID {
		t.Fatalf("short id = %q, want %q", again[0].ShortID, results[0].ShortID)
	}

	id, err := store.Save("http://example.com", testUserID)
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if id == "" {
		t.Fatal("Save returned empty id")
	}
}

func TestFileStorageSaveDuplicateDoesNotAddRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "storage.json")

	store, err := NewFile(path)
	if err != nil {
		t.Fatalf("NewFile returned error: %v", err)
	}

	id, err := store.Save("http://yandex.ru", testUserID)
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	sameID, err := store.Save("http://yandex.ru", otherUserID)
	if !errors.Is(err, ErrURLExists) {
		t.Fatalf("Save existing URL returned %v, want %v", err, ErrURLExists)
	}
	if sameID != id {
		t.Fatalf("sameID = %q, want %q", sameID, id)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}

	var records []fileRecord
	if err := json.Unmarshal(data, &records); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records len = %d, want %d", len(records), 1)
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
	}, testUserID)
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
	if records[0].UUID != "1" || records[0].ShortURL != results[0].ShortID || records[0].OriginalURL != "http://yandex.ru" || records[0].UserID != testUserID {
		t.Fatalf("first record = %+v", records[0])
	}
	if records[1].UUID != "2" || records[1].ShortURL != results[1].ShortID || records[1].OriginalURL != "http://ya.ru" || records[1].UserID != testUserID {
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

	userURLs, err := loaded.FindByUserID(testUserID)
	if err != nil {
		t.Fatalf("FindByUserID returned error: %v", err)
	}
	if len(userURLs) != 2 {
		t.Fatalf("user urls len = %d, want %d", len(userURLs), 2)
	}

	again, err := loaded.SaveBatch([]BatchItem{
		{CorrelationID: "3", OriginalURL: "http://yandex.ru"},
	}, otherUserID)
	if err != nil {
		t.Fatalf("SaveBatch existing URL returned error: %v", err)
	}
	if again[0].ShortID != results[0].ShortID {
		t.Fatalf("short id = %q, want %q", again[0].ShortID, results[0].ShortID)
	}

	id, err := loaded.Save("http://example.com", testUserID)
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if id == "" {
		t.Fatal("Save returned empty id")
	}
}

func TestFileStorageDeleteUserURLs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "storage.json")

	store, err := NewFile(path)
	if err != nil {
		t.Fatalf("NewFile returned error: %v", err)
	}

	id, err := store.Save("http://yandex.ru", testUserID)
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	otherID, err := store.Save("http://example.com", otherUserID)
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	if err := store.DeleteUserURLs([]string{id, otherID}, testUserID); err != nil {
		t.Fatalf("DeleteUserURLs returned error: %v", err)
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
	if !records[0].Deleted {
		t.Fatalf("first record deleted = false, want true: %+v", records[0])
	}
	if records[1].Deleted {
		t.Fatalf("second record deleted = true, want false: %+v", records[1])
	}

	loaded, err := NewFile(path)
	if err != nil {
		t.Fatalf("NewFile returned error: %v", err)
	}

	if _, err := loaded.Find(id); !errors.Is(err, ErrDeleted) {
		t.Fatalf("Find deleted returned %v, want %v", err, ErrDeleted)
	}
	if got, err := loaded.Find(otherID); err != nil || got != "http://example.com" {
		t.Fatalf("Find other = %q, %v", got, err)
	}

	urls, err := loaded.FindByUserID(testUserID)
	if err != nil {
		t.Fatalf("FindByUserID returned error: %v", err)
	}
	if len(urls) != 0 {
		t.Fatalf("user urls = %+v, want empty", urls)
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

func TestFileStorageLoadsOldRecordsWithoutUserID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "storage.json")
	data := []byte(`[{"uuid":"1","short_url":"abc12345","original_url":"http://yandex.ru"}]`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	store, err := NewFile(path)
	if err != nil {
		t.Fatalf("NewFile returned error: %v", err)
	}

	originalURL, err := store.Find("abc12345")
	if err != nil {
		t.Fatalf("Find returned error: %v", err)
	}
	if originalURL != "http://yandex.ru" {
		t.Fatalf("originalURL = %q, want %q", originalURL, "http://yandex.ru")
	}

	urls, err := store.FindByUserID(testUserID)
	if err != nil {
		t.Fatalf("FindByUserID returned error: %v", err)
	}
	if len(urls) != 0 {
		t.Fatalf("urls len = %d, want %d", len(urls), 0)
	}
}
