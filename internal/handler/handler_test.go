package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/true18/shortener/internal/audit"
	"github.com/true18/shortener/internal/auth"
	"github.com/true18/shortener/internal/handler"
	"github.com/true18/shortener/internal/repository"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	zapobserver "go.uber.org/zap/zaptest/observer"
)

const testBaseURL = "http://localhost:8080"
const testUserID = "user-1"

type fakeRepo struct {
	idByURL    map[string]string
	urlByID    map[string]string
	userByID   map[string]string
	deleted    map[string]bool
	order      []string
	next       int
	statsCalls int
	statsErr   error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		idByURL:  make(map[string]string),
		urlByID:  make(map[string]string),
		userByID: make(map[string]string),
		deleted:  make(map[string]bool),
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
	if f.deleted[id] {
		return "", repository.ErrDeleted
	}

	return originalURL, nil
}

func (f *fakeRepo) FindByUserID(userID string) ([]repository.UserURL, error) {
	urls := make([]repository.UserURL, 0)
	for _, id := range f.order {
		if f.userByID[id] != userID || f.deleted[id] {
			continue
		}

		urls = append(urls, repository.UserURL{
			ShortID:     id,
			OriginalURL: f.urlByID[id],
		})
	}

	return urls, nil
}

func (f *fakeRepo) DeleteUserURLs(ids []string, userID string) error {
	for _, id := range ids {
		if f.userByID[id] == userID {
			f.deleted[id] = true
		}
	}

	return nil
}

func (f *fakeRepo) Stats() (repository.Stats, error) {
	f.statsCalls++
	if f.statsErr != nil {
		return repository.Stats{}, f.statsErr
	}

	users := make(map[string]struct{})
	for _, userID := range f.userByID {
		if userID != "" {
			users[userID] = struct{}{}
		}
	}
	return repository.Stats{URLs: int64(len(f.urlByID)), Users: int64(len(users))}, nil
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

func newHandler(repo repository.Store) http.Handler {
	return handler.New(repo, testBaseURL, nil, immediateDeleteQueue{repo: repo})
}

func newHandlerWithPinger(repo repository.Store, pinger handler.Pinger) http.Handler {
	return handler.New(repo, testBaseURL, pinger, immediateDeleteQueue{repo: repo})
}

func newHandlerWithAudit(repo repository.Store, observer audit.Observer) http.Handler {
	return handler.NewWithAudit(
		repo,
		testBaseURL,
		nil,
		immediateDeleteQueue{repo: repo},
		observer,
		zap.NewNop(),
	)
}

func newHandlerWithTrustedSubnet(repo repository.Store, cidr string) http.Handler {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(err)
	}

	return handler.NewWithAuditAndTrustedSubnet(
		repo,
		testBaseURL,
		nil,
		immediateDeleteQueue{repo: repo},
		nil,
		zap.NewNop(),
		network,
	)
}

func TestInternalStatsAccess(t *testing.T) {
	tests := []struct {
		name       string
		cidr       string
		realIP     string
		wantStatus int
	}{
		{name: "IPv4 inside", cidr: "192.168.1.0/24", realIP: "192.168.1.42", wantStatus: http.StatusOK},
		{name: "IPv4 outside", cidr: "192.168.1.0/24", realIP: "192.168.2.42", wantStatus: http.StatusForbidden},
		{name: "IPv6 inside", cidr: "2001:db8::/32", realIP: "2001:db8::1", wantStatus: http.StatusOK},
		{name: "IPv6 outside", cidr: "2001:db8::/32", realIP: "2001:db9::1", wantStatus: http.StatusForbidden},
		{name: "missing header", cidr: "192.168.1.0/24", wantStatus: http.StatusForbidden},
		{name: "empty header", cidr: "192.168.1.0/24", realIP: " ", wantStatus: http.StatusForbidden},
		{name: "invalid IP", cidr: "192.168.1.0/24", realIP: "not-an-ip", wantStatus: http.StatusForbidden},
		{name: "mapped IPv4", cidr: "192.168.1.0/24", realIP: "::ffff:192.168.1.42", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			_, _ = repo.Save("https://example.com/one", "user-1")
			_, _ = repo.Save("https://example.com/two", "user-2")
			h := newHandlerWithTrustedSubnet(repo, tt.cidr)
			req := httptest.NewRequest(http.MethodGet, "/api/internal/stats", nil)
			if tt.realIP != "" {
				req.Header.Set("X-Real-IP", tt.realIP)
			}
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			wantCalls := 0
			if tt.wantStatus == http.StatusOK {
				wantCalls = 1
				if got := rec.Header().Get("Content-Type"); got != "application/json" {
					t.Fatalf("Content-Type = %q", got)
				}
				var got repository.Stats
				if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
					t.Fatalf("Decode: %v", err)
				}
				if got.URLs != 2 || got.Users != 2 {
					t.Fatalf("stats = %+v", got)
				}
			}
			if repo.statsCalls != wantCalls {
				t.Fatalf("Stats calls = %d, want %d", repo.statsCalls, wantCalls)
			}
		})
	}
}

func TestInternalStatsStorageError(t *testing.T) {
	_, network, err := net.ParseCIDR("127.0.0.0/8")
	if err != nil {
		t.Fatalf("ParseCIDR: %v", err)
	}
	h := handler.NewWithAuditAndTrustedSubnet(
		errorRepo{err: errors.New("storage failed")},
		testBaseURL,
		nil,
		nil,
		nil,
		zap.NewNop(),
		network,
	)
	req := httptest.NewRequest(http.MethodGet, "/api/internal/stats", nil)
	req.Header.Set("X-Real-IP", "127.0.0.1")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestInternalStatsDisabledWithoutTrustedSubnet(t *testing.T) {
	repo := newFakeRepo()
	h := newHandler(repo)
	req := httptest.NewRequest(http.MethodGet, "/api/internal/stats", nil)
	req.Header.Set("X-Real-IP", "127.0.0.1")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if repo.statsCalls != 0 {
		t.Fatalf("Stats calls = %d, want 0", repo.statsCalls)
	}
}

type immediateDeleteQueue struct {
	repo repository.Store
}

func (q immediateDeleteQueue) EnqueueDelete(userID string, ids []string) error {
	return q.repo.DeleteUserURLs(ids, userID)
}

type recordingDeleteQueue struct {
	calls chan deleteCall
	err   error
}

type deleteCall struct {
	userID string
	ids    []string
}

func newRecordingDeleteQueue() *recordingDeleteQueue {
	return &recordingDeleteQueue{
		calls: make(chan deleteCall, 1),
	}
}

func (q *recordingDeleteQueue) EnqueueDelete(userID string, ids []string) error {
	if q.err != nil {
		return q.err
	}

	q.calls <- deleteCall{
		userID: userID,
		ids:    append([]string(nil), ids...),
	}

	return nil
}

func TestCreate(t *testing.T) {
	repo := newFakeRepo()
	h := newHandler(repo)

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
	h := newHandler(repo)

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
	h := newHandler(repo)

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
	h := newHandler(repo)

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

func TestAuditSuccessfulHandlers(t *testing.T) {
	repo := newFakeRepo()
	observer := &recordingAuditObserver{}
	h := newHandlerWithAudit(repo, observer)
	started := time.Now().Unix()

	plainReq := withUser(httptest.NewRequest(
		http.MethodPost,
		"/",
		strings.NewReader("https://example.com/plain"),
	))
	plainRec := httptest.NewRecorder()
	h.ServeHTTP(plainRec, plainReq)
	if plainRec.Code != http.StatusCreated {
		t.Fatalf("plain status = %d, want %d", plainRec.Code, http.StatusCreated)
	}

	jsonReq := withUser(httptest.NewRequest(
		http.MethodPost,
		"/api/shorten",
		strings.NewReader(`{"url":"https://example.com/json"}`),
	))
	jsonRec := httptest.NewRecorder()
	h.ServeHTTP(jsonRec, jsonReq)
	if jsonRec.Code != http.StatusCreated {
		t.Fatalf("JSON status = %d, want %d", jsonRec.Code, http.StatusCreated)
	}

	id := strings.TrimPrefix(plainRec.Body.String(), testBaseURL+"/")
	followReq := withUser(httptest.NewRequest(http.MethodGet, "/"+id, nil))
	followRec := httptest.NewRecorder()
	h.ServeHTTP(followRec, followReq)
	if followRec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("follow status = %d, want %d", followRec.Code, http.StatusTemporaryRedirect)
	}

	events := observer.Events()
	want := []audit.Event{
		{Action: audit.ActionShorten, UserID: testUserID, URL: "https://example.com/plain"},
		{Action: audit.ActionShorten, UserID: testUserID, URL: "https://example.com/json"},
		{Action: audit.ActionFollow, UserID: testUserID, URL: "https://example.com/plain"},
	}
	if len(events) != len(want) {
		t.Fatalf("events = %+v, want %d events", events, len(want))
	}
	for i := range want {
		if events[i].Action != want[i].Action || events[i].UserID != want[i].UserID || events[i].URL != want[i].URL {
			t.Fatalf("event %d = %+v, want %+v", i, events[i], want[i])
		}
		if events[i].Timestamp < started || events[i].Timestamp > time.Now().Unix() {
			t.Fatalf("event %d timestamp = %d", i, events[i].Timestamp)
		}
	}
}

func TestAuditSkipsDuplicateAndFailedRequests(t *testing.T) {
	repo := newFakeRepo()
	if _, err := repo.Save("https://example.com/duplicate", testUserID); err != nil {
		t.Fatalf("Save: %v", err)
	}
	observer := &recordingAuditObserver{}
	h := newHandlerWithAudit(repo, observer)

	duplicateReq := withUser(httptest.NewRequest(
		http.MethodPost,
		"/",
		strings.NewReader("https://example.com/duplicate"),
	))
	duplicateRec := httptest.NewRecorder()
	h.ServeHTTP(duplicateRec, duplicateReq)
	if duplicateRec.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d, want %d", duplicateRec.Code, http.StatusConflict)
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/missing", nil)
	missingRec := httptest.NewRecorder()
	h.ServeHTTP(missingRec, missingReq)

	id, err := repo.Save("https://example.com/deleted", testUserID)
	if err != nil {
		t.Fatalf("Save deleted URL: %v", err)
	}
	repo.deleted[id] = true
	deletedReq := httptest.NewRequest(http.MethodGet, "/"+id, nil)
	deletedRec := httptest.NewRecorder()
	h.ServeHTTP(deletedRec, deletedReq)
	if deletedRec.Code != http.StatusGone {
		t.Fatalf("deleted status = %d, want %d", deletedRec.Code, http.StatusGone)
	}

	if events := observer.Events(); len(events) != 0 {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestAuditSkipsJSONDuplicate(t *testing.T) {
	repo := newFakeRepo()
	if _, err := repo.Save("https://example.com/duplicate", testUserID); err != nil {
		t.Fatalf("Save: %v", err)
	}
	observer := &recordingAuditObserver{}
	h := newHandlerWithAudit(repo, observer)
	req := withUser(httptest.NewRequest(
		http.MethodPost,
		"/api/shorten",
		strings.NewReader(`{"url":"https://example.com/duplicate"}`),
	))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
	if events := observer.Events(); len(events) != 0 {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestAuditStorageFailureDoesNotPublish(t *testing.T) {
	observer := &recordingAuditObserver{}
	h := newHandlerWithAudit(errorRepo{err: errors.New("save failed")}, observer)
	req := withUser(httptest.NewRequest(http.MethodPost, "/", strings.NewReader("https://example.com")))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if events := observer.Events(); len(events) != 0 {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestAuditFailureDoesNotChangeResponse(t *testing.T) {
	observer := &recordingAuditObserver{err: errors.New("audit failed")}
	h := newHandlerWithAudit(newFakeRepo(), observer)
	req := withUser(httptest.NewRequest(http.MethodPost, "/", strings.NewReader("https://example.com")))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if len(observer.Events()) != 1 {
		t.Fatalf("events = %+v, want one", observer.Events())
	}
}

func TestAuditQueueDoesNotDelayHandlersOrUseRequestContext(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		original   string
		wantAction string
		wantStatus int
	}{
		{
			name:       "plain shorten",
			method:     http.MethodPost,
			path:       "/",
			body:       "https://example.com/plain",
			original:   "https://example.com/plain",
			wantAction: audit.ActionShorten,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "JSON shorten",
			method:     http.MethodPost,
			path:       "/api/shorten",
			body:       `{"url":"https://example.com/json"}`,
			original:   "https://example.com/json",
			wantAction: audit.ActionShorten,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "redirect",
			method:     http.MethodGet,
			path:       "/existing1",
			original:   "https://example.com/redirect",
			wantAction: audit.ActionFollow,
			wantStatus: http.StatusTemporaryRedirect,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := repository.NewMemory()
			if tt.method == http.MethodGet {
				id, err := repo.Save(tt.original, testUserID)
				if err != nil {
					t.Fatalf("Save existing URL: %v", err)
				}
				tt.path = "/" + id
			}

			entered := make(chan audit.Event, 1)
			release := make(chan struct{})
			var releaseOnce sync.Once
			observer := testObserverFunc(func(ctx context.Context, event audit.Event) error {
				entered <- event
				select {
				case <-release:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			})
			queue := audit.NewQueue(observer, zap.NewNop())
			t.Cleanup(func() {
				releaseOnce.Do(func() { close(release) })
				_ = queue.Close()
			})

			h := handler.NewWithAudit(repo, testBaseURL, nil, nil, queue, zap.NewNop())
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			ctx, cancel := context.WithCancel(auth.WithUserID(req.Context(), testUserID))
			defer cancel()
			req = req.WithContext(ctx)
			response := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				response <- rec
			}()

			var rec *httptest.ResponseRecorder
			select {
			case rec = <-response:
			case <-time.After(time.Second):
				t.Fatal("handler waited for the audit observer")
			}
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			var event audit.Event
			select {
			case event = <-entered:
			case <-time.After(time.Second):
				t.Fatal("audit worker did not start delivery")
			}
			cancel()
			if event.Action != tt.wantAction || event.URL != tt.original || event.UserID != testUserID {
				t.Fatalf("event = %+v, want action %q, URL %q, and user %q", event, tt.wantAction, tt.original, testUserID)
			}
			releaseOnce.Do(func() { close(release) })
			if err := queue.Close(); err != nil {
				t.Fatalf("Close audit queue: %v", err)
			}
		})
	}
}

func TestFullAuditQueueIsLoggedWithoutChangingResponse(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	blocked := testObserverFunc(func(context.Context, audit.Event) error {
		startOnce.Do(func() { close(started) })
		<-release
		return nil
	})
	queue := audit.NewQueue(blocked, zap.NewNop())
	t.Cleanup(func() {
		close(release)
		_ = queue.Close()
	})
	if err := queue.Notify(context.Background(), audit.Event{}); err != nil {
		t.Fatalf("initial Notify: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("audit worker did not start")
	}
	for {
		err := queue.Notify(context.Background(), audit.Event{})
		if errors.Is(err, audit.ErrQueueFull) {
			break
		}
		if err != nil {
			t.Fatalf("filling audit queue: %v", err)
		}
	}

	logCore, logs := zapobserver.New(zapcore.ErrorLevel)
	h := handler.NewWithAudit(newFakeRepo(), testBaseURL, nil, nil, queue, zap.New(logCore))
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("https://example.com/queued"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withUser(req))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if logs.FilterMessage("audit delivery failed").Len() == 0 {
		t.Fatal("full audit queue error was not logged")
	}
}

func TestFollowAuditOmitsUnavailableUserID(t *testing.T) {
	repo := newFakeRepo()
	id, err := repo.Save("https://example.com", testUserID)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	observer := &recordingAuditObserver{}
	h := newHandlerWithAudit(repo, observer)
	req := httptest.NewRequest(http.MethodGet, "/"+id, nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTemporaryRedirect)
	}
	events := observer.Events()
	if len(events) != 1 || events[0].UserID != "" {
		t.Fatalf("events = %+v, want event without user ID", events)
	}
}

func TestCreateBatch(t *testing.T) {
	repo := newFakeRepo()
	h := newHandler(repo)

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
	h := newHandler(repo)

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
	h := newHandler(repo)

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
			h := newHandlerWithPinger(repo, tt.pinger)

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
	h := newHandler(newFakeRepo())
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
	h := newHandler(repo)

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
	h := newHandler(newFakeRepo())
	req := httptest.NewRequest(http.MethodGet, "/api/user/urls", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestUserURLsBadMethod(t *testing.T) {
	h := newHandler(newFakeRepo())
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/user/urls", nil))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteUserURLs(t *testing.T) {
	repo := newFakeRepo()
	id, err := repo.Save("https://practicum.yandex.ru", testUserID)
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	h := newHandler(repo)

	req := withUser(httptest.NewRequest(http.MethodDelete, "/api/user/urls", strings.NewReader(`["  `+id+`  "]`)))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	redirectReq := httptest.NewRequest(http.MethodGet, "/"+id, nil)
	redirectRec := httptest.NewRecorder()
	h.ServeHTTP(redirectRec, redirectReq)

	if redirectRec.Code != http.StatusGone {
		t.Fatalf("redirect status = %d, want %d", redirectRec.Code, http.StatusGone)
	}

	listReq := withUser(httptest.NewRequest(http.MethodGet, "/api/user/urls", nil))
	listRec := httptest.NewRecorder()
	h.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusNoContent {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusNoContent)
	}
}

func TestDeleteUserURLsEnqueues(t *testing.T) {
	repo := newFakeRepo()
	id, err := repo.Save("https://practicum.yandex.ru", testUserID)
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	queue := newRecordingDeleteQueue()
	h := handler.New(repo, testBaseURL, nil, queue)

	req := withUser(httptest.NewRequest(http.MethodDelete, "/api/user/urls", strings.NewReader(`["`+id+`"]`)))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	select {
	case call := <-queue.calls:
		if call.userID != testUserID {
			t.Fatalf("user id = %q, want %q", call.userID, testUserID)
		}
		if len(call.ids) != 1 || call.ids[0] != id {
			t.Fatalf("ids = %+v, want [%q]", call.ids, id)
		}
	case <-time.After(time.Second):
		t.Fatal("delete task was not enqueued")
	}

	if got, err := repo.Find(id); err != nil || got != "https://practicum.yandex.ru" {
		t.Fatalf("Find after enqueue = %q, %v", got, err)
	}
}

func TestDeleteUserURLsDoesNotDeleteOtherUserURL(t *testing.T) {
	repo := newFakeRepo()
	id, err := repo.Save("https://practicum.yandex.ru", "other-user")
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	h := newHandler(repo)

	req := withUser(httptest.NewRequest(http.MethodDelete, "/api/user/urls", strings.NewReader(`["`+id+`"]`)))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	redirectReq := httptest.NewRequest(http.MethodGet, "/"+id, nil)
	redirectRec := httptest.NewRecorder()
	h.ServeHTTP(redirectRec, redirectReq)

	if redirectRec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("redirect status = %d, want %d", redirectRec.Code, http.StatusTemporaryRedirect)
	}
}

func TestDeleteUserURLsUnauthorized(t *testing.T) {
	h := newHandler(newFakeRepo())
	req := httptest.NewRequest(http.MethodDelete, "/api/user/urls", strings.NewReader(`["abc12345"]`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestDeleteUserURLsBadRequests(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "invalid json",
			body: "{",
		},
		{
			name: "not array",
			body: `{"id":"abc12345"}`,
		},
		{
			name: "empty array",
			body: `[]`,
		},
		{
			name: "empty id",
			body: `[""]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHandler(newFakeRepo())
			req := withUser(httptest.NewRequest(http.MethodDelete, "/api/user/urls", strings.NewReader(tt.body)))
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
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
			h := newHandler(newFakeRepo())
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
			h := newHandler(newFakeRepo())
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
			h := newHandler(newFakeRepo())
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
	h := newHandler(errorRepo{err: errStorage})

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
		{
			name:   "delete error",
			method: http.MethodDelete,
			target: "/api/user/urls",
			body:   `["abc12345"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			if tt.method == http.MethodPost || tt.method == http.MethodDelete {
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

type recordingAuditObserver struct {
	mu     sync.Mutex
	events []audit.Event
	err    error
}

type testObserverFunc func(context.Context, audit.Event) error

func (f testObserverFunc) Notify(ctx context.Context, event audit.Event) error {
	return f(ctx, event)
}

func (o *recordingAuditObserver) Notify(_ context.Context, event audit.Event) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, event)

	return o.err
}

func (o *recordingAuditObserver) Events() []audit.Event {
	o.mu.Lock()
	defer o.mu.Unlock()

	return append([]audit.Event(nil), o.events...)
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

func (r errorRepo) DeleteUserURLs([]string, string) error {
	return r.err
}

func (r errorRepo) Stats() (repository.Stats, error) {
	return repository.Stats{}, r.err
}

func withUser(req *http.Request) *http.Request {
	return req.WithContext(auth.WithUserID(req.Context(), testUserID))
}
