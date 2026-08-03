package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/true18/shortener/internal/handler"
	"github.com/true18/shortener/internal/repository"
)

const testBaseURL = "http://localhost:8080"

type fakeRepo struct {
	idByURL map[string]string
	urlByID map[string]string
	nextID  string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		idByURL: make(map[string]string),
		urlByID: make(map[string]string),
		nextID:  "abc12345",
	}
}

func (f *fakeRepo) Save(originalURL string) (string, error) {
	if id, ok := f.idByURL[originalURL]; ok {
		return id, nil
	}

	f.idByURL[originalURL] = f.nextID
	f.urlByID[f.nextID] = originalURL

	return f.nextID, nil
}

func (f *fakeRepo) Find(id string) (string, error) {
	originalURL, ok := f.urlByID[id]
	if !ok {
		return "", repository.ErrNotFound
	}

	return originalURL, nil
}

type fakePinger struct {
	err error
}

func (p fakePinger) PingContext(context.Context) error {
	return p.err
}

func TestCreate(t *testing.T) {
	repo := newFakeRepo()
	h := handler.New(repo, testBaseURL, nil)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("https://practicum.yandex.ru/"))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain" {
		t.Fatalf("Content-Type = %q, want %q", got, "text/plain")
	}
	if got := rec.Body.String(); got != testBaseURL+"/abc12345" {
		t.Fatalf("body = %q, want %q", got, testBaseURL+"/abc12345")
	}
	if _, ok := repo.idByURL["https://practicum.yandex.ru/"]; !ok {
		t.Fatal("original URL was not saved")
	}
}

func TestCreateJSON(t *testing.T) {
	repo := newFakeRepo()
	h := handler.New(repo, testBaseURL, nil)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/shorten",
		strings.NewReader(`{"url":"https://practicum.yandex.ru"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var resp struct {
		Result string `json:"result"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if resp.Result == "" {
		t.Fatal("result is empty")
	}
	if !strings.HasPrefix(resp.Result, testBaseURL+"/") {
		t.Fatalf("result = %q, want prefix %q", resp.Result, testBaseURL+"/")
	}

	id := strings.TrimPrefix(resp.Result, testBaseURL+"/")
	redirectReq := httptest.NewRequest(http.MethodGet, "/"+id, nil)
	redirectRec := httptest.NewRecorder()

	h.ServeHTTP(redirectRec, redirectReq)

	if redirectRec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("redirect status = %d, want %d", redirectRec.Code, http.StatusTemporaryRedirect)
	}
	if got := redirectRec.Header().Get("Location"); got != "https://practicum.yandex.ru" {
		t.Fatalf("Location = %q, want %q", got, "https://practicum.yandex.ru")
	}
}

func TestRedirect(t *testing.T) {
	repo := newFakeRepo()
	repo.urlByID["abc12345"] = "https://practicum.yandex.ru/"
	h := handler.New(repo, testBaseURL, nil)

	req := httptest.NewRequest(http.MethodGet, "/abc12345", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTemporaryRedirect)
	}
	if got := rec.Header().Get("Location"); got != "https://practicum.yandex.ru/" {
		t.Fatalf("Location = %q, want %q", got, "https://practicum.yandex.ru/")
	}
}

func TestPing(t *testing.T) {
	tests := []struct {
		name   string
		pinger handler.Pinger
		method string
		want   int
	}{
		{
			name:   "ok",
			pinger: fakePinger{},
			method: http.MethodGet,
			want:   http.StatusOK,
		},
		{
			name:   "ping error",
			pinger: fakePinger{err: errors.New("db failed")},
			method: http.MethodGet,
			want:   http.StatusInternalServerError,
		},
		{
			name:   "no connection",
			method: http.MethodGet,
			want:   http.StatusInternalServerError,
		},
		{
			name:   "unsupported method",
			pinger: fakePinger{},
			method: http.MethodPost,
			want:   http.StatusBadRequest,
		},
		{
			name:   "not short id",
			pinger: fakePinger{},
			method: http.MethodGet,
			want:   http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			repo.urlByID["ping"] = "https://practicum.yandex.ru/"
			h := handler.New(repo, testBaseURL, tt.pinger)

			req := httptest.NewRequest(tt.method, "/ping", nil)
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
			if rec.Code == http.StatusTemporaryRedirect {
				t.Fatal("/ping was handled as short URL")
			}
		})
	}
}

func TestCreateJSONBadRequests(t *testing.T) {
	tests := []struct {
		name   string
		method string
		body   string
	}{
		{
			name:   "invalid json",
			method: http.MethodPost,
			body:   "{",
		},
		{
			name:   "missing url",
			method: http.MethodPost,
			body:   `{}`,
		},
		{
			name:   "empty url",
			method: http.MethodPost,
			body:   `{"url":""}`,
		},
		{
			name:   "invalid url",
			method: http.MethodPost,
			body:   `{"url":"ftp://example.com/"}`,
		},
		{
			name:   "unsupported method",
			method: http.MethodGet,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := handler.New(newFakeRepo(), testBaseURL, nil)
			req := httptest.NewRequest(tt.method, "/api/shorten", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestBadRequests(t *testing.T) {
	tests := []struct {
		name   string
		method string
		target string
		body   string
	}{
		{
			name:   "unsupported method",
			method: http.MethodPut,
			target: "/",
		},
		{
			name:   "empty post body",
			method: http.MethodPost,
			target: "/",
		},
		{
			name:   "post to id path",
			method: http.MethodPost,
			target: "/abc12345",
			body:   "https://practicum.yandex.ru/",
		},
		{
			name:   "get without id",
			method: http.MethodGet,
			target: "/",
		},
		{
			name:   "unknown id",
			method: http.MethodGet,
			target: "/unknown",
		},
		{
			name:   "nested path",
			method: http.MethodGet,
			target: "/abc/def",
		},
		{
			name:   "relative url",
			method: http.MethodPost,
			target: "/",
			body:   "practicum.yandex.ru",
		},
		{
			name:   "unsupported url scheme",
			method: http.MethodPost,
			target: "/",
			body:   "ftp://example.com/",
		},
		{
			name:   "url without host",
			method: http.MethodPost,
			target: "/",
			body:   "https:///path",
		},
		{
			name:   "url with whitespace",
			method: http.MethodPost,
			target: "/",
			body:   "https://example .com/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := handler.New(newFakeRepo(), testBaseURL, nil)
			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestStorageErrors(t *testing.T) {
	errStorage := errors.New("storage failed")
	h := handler.New(errorRepo{err: errStorage}, testBaseURL, nil)

	tests := []struct {
		name   string
		method string
		target string
		body   string
	}{
		{
			name:   "save error",
			method: http.MethodPost,
			target: "/",
			body:   "https://practicum.yandex.ru/",
		},
		{
			name:   "find error",
			method: http.MethodGet,
			target: "/abc12345",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
			}
		})
	}
}

type errorRepo struct {
	err error
}

func (r errorRepo) Save(string) (string, error) {
	return "", r.err
}

func (r errorRepo) Find(string) (string, error) {
	return "", r.err
}
