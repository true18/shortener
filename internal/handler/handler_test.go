package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/true18/shortener/internal/auth"
	"github.com/true18/shortener/internal/handler"
	"github.com/true18/shortener/internal/repository"
)

const testBaseURL = "http://localhost:8080"
const testUserID = "user-1"

type fakeRepo struct {
	idByURL  map[string]string
	urlByID  map[string]string
	userByID map[string]string
	order    []string
	next     int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		idByURL:  make(map[string]string),
		urlByID:  make(map[string]string),
		userByID: make(map[string]string),
	}
}

func (f *fakeRepo) Save(originalURL string, userID string) (string, error) {
	if id, ok := f.idByURL[originalURL]; ok {
		return id, repository.ErrURLExists
	}

	id := f.nextID()
	f.idByURL[originalURL] = id
	f.urlByID[id] = originalURL
	f.userByID[id] = userID
	f.order = append(f.order, id)

	return id, nil
}

func (f *fakeRepo) SaveBatch(items []repository.BatchItem, userID string) ([]repository.BatchResult, error) {
	results := make([]repository.BatchResult, len(items))
	for i, item := range items {
		id, ok := f.idByURL[item.OriginalURL]
		if !ok {
			id = f.nextID()
			f.idByURL[item.OriginalURL] = id
			f.urlByID[id] = item.OriginalURL
			f.userByID[id] = userID
			f.order = append(f.order, id)
		}

		results[i] = repository.BatchResult{
			CorrelationID: item.CorrelationID,
			ShortID:       id,
		}
	}

	return results, nil
}

func (f *fakeRepo) Find(id string) (string, error) {
	originalURL, ok := f.urlByID[id]
	if !ok {
		return "", repository.ErrNotFound
	}

	return originalURL, nil
}

func (f *fakeRepo) FindByUserID(userID string) ([]repository.UserURL, error) {
	urls := make([]repository.UserURL, 0)
	for _, id := range f.order {
		if f.userByID[id] != userID {
			continue
		}

		urls = append(urls, repository.UserURL{
			ShortID:     id,
			OriginalURL: f.urlByID[id],
		})
	}

	return urls, nil
}

func (f *fakeRepo) nextID() string {
	if f.next == 0 {
		f.next++
		return "abc12345"
	}

	id := fmt.Sprintf("id%06d", f.next)
	f.next++

	return id
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

	h.ServeHTTP(rec, withUser(req))

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

func TestCreateDuplicate(t *testing.T) {
	repo := newFakeRepo()
	h := handler.New(repo, testBaseURL, nil)

	firstReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("https://practicum.yandex.ru/"))
	firstRec := httptest.NewRecorder()
	h.ServeHTTP(firstRec, withUser(firstReq))

	if firstRec.Code != http.StatusCreated {
		t.Fatalf("first status = %d, want %d", firstRec.Code, http.StatusCreated)
	}
	shortURL := firstRec.Body.String()

	secondReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("https://practicum.yandex.ru/"))
	secondRec := httptest.NewRecorder()
	h.ServeHTTP(secondRec, withUser(secondReq))

	if secondRec.Code != http.StatusConflict {
		t.Fatalf("second status = %d, want %d", secondRec.Code, http.StatusConflict)
	}
	if got := secondRec.Header().Get("Content-Type"); got != "text/plain" {
		t.Fatalf("Content-Type = %q, want %q", got, "text/plain")
	}
	if got := secondRec.Body.String(); got != shortURL {
		t.Fatalf("body = %q, want %q", got, shortURL)
	}

	id := strings.TrimPrefix(shortURL, testBaseURL+"/")
	redirectReq := httptest.NewRequest(http.MethodGet, "/"+id, nil)
	redirectRec := httptest.NewRecorder()
	h.ServeHTTP(redirectRec, redirectReq)

	if redirectRec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("redirect status = %d, want %d", redirectRec.Code, http.StatusTemporaryRedirect)
	}
	if got := redirectRec.Header().Get("Location"); got != "https://practicum.yandex.ru/" {
		t.Fatalf("Location = %q, want %q", got, "https://practicum.yandex.ru/")
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

	h.ServeHTTP(rec, withUser(req))

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

func TestCreateJSONDuplicate(t *testing.T) {
	repo := newFakeRepo()
	h := handler.New(repo, testBaseURL, nil)

	firstReq := httptest.NewRequest(
		http.MethodPost,
		"/api/shorten",
		strings.NewReader(`{"url":"https://practicum.yandex.ru"}`),
	)
	firstRec := httptest.NewRecorder()
	h.ServeHTTP(firstRec, withUser(firstReq))

	if firstRec.Code != http.StatusCreated {
		t.Fatalf("first status = %d, want %d", firstRec.Code, http.StatusCreated)
	}
	var firstResp struct {
		Result string `json:"result"`
	}
	if err := json.NewDecoder(firstRec.Body).Decode(&firstResp); err != nil {
		t.Fatalf("Decode first response: %v", err)
	}

	secondReq := httptest.NewRequest(
		http.MethodPost,
		"/api/shorten",
		strings.NewReader(`{"url":"https://practicum.yandex.ru"}`),
	)
	secondRec := httptest.NewRecorder()
	h.ServeHTTP(secondRec, withUser(secondReq))

	if secondRec.Code != http.StatusConflict {
		t.Fatalf("second status = %d, want %d", secondRec.Code, http.StatusConflict)
	}
	if got := secondRec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var secondResp struct {
		Result string `json:"result"`
	}
	if err := json.NewDecoder(secondRec.Body).Decode(&secondResp); err != nil {
		t.Fatalf("Decode second response: %v", err)
	}
	if secondResp.Result != firstResp.Result {
		t.Fatalf("result = %q, want %q", secondResp.Result, firstResp.Result)
	}

	id := strings.TrimPrefix(secondResp.Result, testBaseURL+"/")
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

func TestCreateBatch(t *testing.T) {
	repo := newFakeRepo()
	h := handler.New(repo, testBaseURL, nil)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/shorten/batch",
		strings.NewReader(`[
			{"correlation_id":"1","original_url":"https://practicum.yandex.ru"},
			{"correlation_id":"2","original_url":"https://example.com"}
		]`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, withUser(req))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var resp []struct {
		CorrelationID string `json:"correlation_id"`
		ShortURL      string `json:"short_url"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("response len = %d, want %d", len(resp), 2)
	}
	if resp[0].CorrelationID != "1" || resp[1].CorrelationID != "2" {
		t.Fatalf("correlation ids = %q, %q", resp[0].CorrelationID, resp[1].CorrelationID)
	}
	for _, item := range resp {
		if !strings.HasPrefix(item.ShortURL, testBaseURL+"/") {
			t.Fatalf("short_url = %q, want prefix %q", item.ShortURL, testBaseURL+"/")
		}
	}
}

func TestCreateBatchExistingURL(t *testing.T) {
	repo := newFakeRepo()
	id, err := repo.Save("https://practicum.yandex.ru", testUserID)
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	h := handler.New(repo, testBaseURL, nil)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/shorten/batch",
		strings.NewReader(`[{"correlation_id":"1","original_url":"https://practicum.yandex.ru"}]`),
	)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, withUser(req))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var resp []struct {
		CorrelationID string `json:"correlation_id"`
		ShortURL      string `json:"short_url"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("response len = %d, want %d", len(resp), 1)
	}
	if got := resp[0].ShortURL; got != testBaseURL+"/"+id {
		t.Fatalf("short_url = %q, want %q", got, testBaseURL+"/"+id)
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

func TestUserURLsNoContent(t *testing.T) {
	h := handler.New(newFakeRepo(), testBaseURL, nil)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/user/urls", nil))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestUserURLs(t *testing.T) {
	repo := newFakeRepo()
	id, err := repo.Save("https://practicum.yandex.ru", testUserID)
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	_, err = repo.Save("https://example.com", "other-user")
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	h := handler.New(repo, testBaseURL, nil)

	req := withUser(httptest.NewRequest(http.MethodGet, "/api/user/urls", nil))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var resp []struct {
		ShortURL    string `json:"short_url"`
		OriginalURL string `json:"original_url"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("response len = %d, want %d", len(resp), 1)
	}
	if resp[0].ShortURL != testBaseURL+"/"+id {
		t.Fatalf("short_url = %q, want %q", resp[0].ShortURL, testBaseURL+"/"+id)
	}
	if resp[0].OriginalURL != "https://practicum.yandex.ru" {
		t.Fatalf("original_url = %q", resp[0].OriginalURL)
	}
}

func TestUserURLsUnauthorized(t *testing.T) {
	h := handler.New(newFakeRepo(), testBaseURL, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/user/urls", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestUserURLsBadMethod(t *testing.T) {
	h := handler.New(newFakeRepo(), testBaseURL, nil)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/user/urls", nil))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
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

func TestCreateBatchBadRequests(t *testing.T) {
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
			name:   "not array",
			method: http.MethodPost,
			body:   `{"correlation_id":"1","original_url":"https://example.com"}`,
		},
		{
			name:   "empty array",
			method: http.MethodPost,
			body:   `[]`,
		},
		{
			name:   "missing correlation id",
			method: http.MethodPost,
			body:   `[{"original_url":"https://example.com"}]`,
		},
		{
			name:   "missing original url",
			method: http.MethodPost,
			body:   `[{"correlation_id":"1"}]`,
		},
		{
			name:   "empty original url",
			method: http.MethodPost,
			body:   `[{"correlation_id":"1","original_url":""}]`,
		},
		{
			name:   "invalid url",
			method: http.MethodPost,
			body:   `[{"correlation_id":"1","original_url":"ftp://example.com"}]`,
		},
		{
			name:   "unsupported method",
			method: http.MethodGet,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := handler.New(newFakeRepo(), testBaseURL, nil)
			req := httptest.NewRequest(tt.method, "/api/shorten/batch", strings.NewReader(tt.body))
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
		{
			name:   "save batch error",
			method: http.MethodPost,
			target: "/api/shorten/batch",
			body:   `[{"correlation_id":"1","original_url":"https://practicum.yandex.ru/"}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			if tt.method == http.MethodPost {
				req = withUser(req)
			}
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

func (r errorRepo) Save(string, string) (string, error) {
	return "", r.err
}

func (r errorRepo) SaveBatch([]repository.BatchItem, string) ([]repository.BatchResult, error) {
	return nil, r.err
}

func (r errorRepo) Find(string) (string, error) {
	return "", r.err
}

func (r errorRepo) FindByUserID(string) ([]repository.UserURL, error) {
	return nil, r.err
}

func withUser(req *http.Request) *http.Request {
	return req.WithContext(auth.WithUserID(req.Context(), testUserID))
}
