package audit

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestPublisherNotifiesEveryObserver(t *testing.T) {
	first := &recordingObserver{}
	second := &recordingObserver{}
	publisher := NewPublisher(first, second)
	event := Event{Timestamp: 123, Action: ActionShorten, UserID: "user-1", URL: "https://example.com"}

	if err := publisher.Notify(context.Background(), event); err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}

	for i, observer := range []*recordingObserver{first, second} {
		if got := observer.Events(); len(got) != 1 || got[0] != event {
			t.Fatalf("observer %d events = %+v, want %+v", i+1, got, event)
		}
	}
}

func TestPublisherContinuesAfterObserverError(t *testing.T) {
	wantErr := errors.New("delivery failed")
	failed := &recordingObserver{err: wantErr}
	succeeded := &recordingObserver{}
	publisher := NewPublisher(failed, succeeded)
	event := Event{Action: ActionFollow, URL: "https://example.com"}

	err := publisher.Notify(context.Background(), event)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Notify error = %v, want %v", err, wantErr)
	}
	if got := succeeded.Events(); len(got) != 1 || got[0] != event {
		t.Fatalf("second observer events = %+v, want %+v", got, event)
	}
}

func TestPublisherWithoutObservers(t *testing.T) {
	if err := NewPublisher().Notify(context.Background(), Event{}); err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}
}

func TestFileObserverAppendsJSONLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	if err := os.WriteFile(path, []byte("existing\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	observer, err := NewFileObserver(path)
	if err != nil {
		t.Fatalf("NewFileObserver: %v", err)
	}
	events := []Event{
		{Timestamp: 123, Action: ActionShorten, UserID: "user-1", URL: "https://example.com/one"},
		{Timestamp: 124, Action: ActionFollow, URL: "https://example.com/two"},
	}
	for _, event := range events {
		if err := observer.Notify(context.Background(), event); err != nil {
			t.Fatalf("Notify: %v", err)
		}
	}
	if err := observer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() || scanner.Text() != "existing" {
		t.Fatalf("first line = %q, want existing", scanner.Text())
	}
	for i, want := range events {
		if !scanner.Scan() {
			t.Fatalf("missing event line %d", i+1)
		}
		var got Event
		if err := json.Unmarshal(scanner.Bytes(), &got); err != nil {
			t.Fatalf("Unmarshal line %d: %v", i+1, err)
		}
		if got != want {
			t.Fatalf("event %d = %+v, want %+v", i+1, got, want)
		}
	}
	if scanner.Scan() {
		t.Fatalf("unexpected line %q", scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("Scan: %v", err)
	}
}

func TestFileObserverConcurrentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	observer, err := NewFileObserver(path)
	if err != nil {
		t.Fatalf("NewFileObserver: %v", err)
	}

	const eventCount = 50
	var wg sync.WaitGroup
	for i := 0; i < eventCount; i++ {
		wg.Add(1)
		go func(timestamp int64) {
			defer wg.Done()
			if err := observer.Notify(context.Background(), Event{
				Timestamp: timestamp,
				Action:    ActionShorten,
				URL:       "https://example.com",
			}); err != nil {
				t.Errorf("Notify: %v", err)
			}
		}(int64(i))
	}
	wg.Wait()
	if err := observer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("invalid JSON line %d: %v", count+1, err)
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if count != eventCount {
		t.Fatalf("line count = %d, want %d", count, eventCount)
	}
}

func TestFileObserverReturnsWriteError(t *testing.T) {
	observer, err := NewFileObserver(filepath.Join(t.TempDir(), "audit.jsonl"))
	if err != nil {
		t.Fatalf("NewFileObserver: %v", err)
	}
	if err := observer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := observer.Notify(context.Background(), Event{}); err == nil {
		t.Fatal("Notify returned nil error after Close")
	}
}

func TestHTTPObserverPostsEvent(t *testing.T) {
	want := Event{Timestamp: 123, Action: ActionShorten, UserID: "user-1", URL: "https://example.com"}
	received := make(chan Event, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.RequestURI() != "/events?source=test" {
			t.Errorf("request URI = %q", r.URL.RequestURI())
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		var event Event
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Errorf("Decode: %v", err)
		}
		received <- event
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	observer, err := NewHTTPObserver(server.URL+"/events?source=test", server.Client())
	if err != nil {
		t.Fatalf("NewHTTPObserver: %v", err)
	}
	if err := observer.Notify(context.Background(), want); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if got := <-received; got != want {
		t.Fatalf("event = %+v, want %+v", got, want)
	}
}

func TestHTTPObserverReturnsStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	observer, err := NewHTTPObserver(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewHTTPObserver: %v", err)
	}
	if err := observer.Notify(context.Background(), Event{}); err == nil {
		t.Fatal("Notify returned nil error")
	}
}

func TestPublisherWritesFileWhenHTTPObserverFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	httpObserver, err := NewHTTPObserver(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewHTTPObserver: %v", err)
	}
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	fileObserver, err := NewFileObserver(path)
	if err != nil {
		t.Fatalf("NewFileObserver: %v", err)
	}
	defer fileObserver.Close()

	event := Event{Timestamp: 123, Action: ActionFollow, URL: "https://example.com"}
	if err := NewPublisher(httpObserver, fileObserver).Notify(context.Background(), event); err == nil {
		t.Fatal("Notify returned nil error")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got Event
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got != event {
		t.Fatalf("event = %+v, want %+v", got, event)
	}
}

func TestHTTPObserverTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := server.Client()
	client.Timeout = 10 * time.Millisecond
	observer, err := NewHTTPObserver(server.URL, client)
	if err != nil {
		t.Fatalf("NewHTTPObserver: %v", err)
	}
	if err := observer.Notify(context.Background(), Event{}); err == nil {
		t.Fatal("Notify returned nil error")
	}
}

type recordingObserver struct {
	mu     sync.Mutex
	events []Event
	err    error
}

func (o *recordingObserver) Notify(_ context.Context, event Event) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, event)

	return o.err
}

func (o *recordingObserver) Events() []Event {
	o.mu.Lock()
	defer o.mu.Unlock()

	return append([]Event(nil), o.events...)
}
