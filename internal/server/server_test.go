package server_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/true18/shortener/internal/auth"
	"github.com/true18/shortener/internal/config"
	"github.com/true18/shortener/internal/server"
	"go.uber.org/zap"
)

const testBaseURL = "http://localhost:8080"

const pingDriverName = "shortener_ping_test"

func init() {
	sql.Register(pingDriverName, pingDriver{})
}

func TestShortenJSONWithoutGzip(t *testing.T) {
	h := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/shorten", strings.NewReader(`{"url":"https://practicum.yandex.ru"}`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}

	var resp struct {
		Result string `json:"result"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
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

func TestShortenJSONWithGzipResponse(t *testing.T) {
	h := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/shorten", strings.NewReader(`{"url":"https://practicum.yandex.ru"}`))
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want %q", got, "gzip")
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var resp struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(readGzip(t, rec.Body), &resp); err != nil {
		t.Fatalf("Unmarshal response: %v", err)
	}
	if !strings.HasPrefix(resp.Result, testBaseURL+"/") {
		t.Fatalf("result = %q, want prefix %q", resp.Result, testBaseURL+"/")
	}
}

func TestShortenJSONWithGzipRequest(t *testing.T) {
	h := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/shorten", gzipBody(t, `{"url":"https://practicum.yandex.ru"}`))
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var resp struct {
		Result string `json:"result"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if !strings.HasPrefix(resp.Result, testBaseURL+"/") {
		t.Fatalf("result = %q, want prefix %q", resp.Result, testBaseURL+"/")
	}
}

func TestShortenBatchWithGzipResponse(t *testing.T) {
	h := newTestServer(t)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/shorten/batch",
		strings.NewReader(`[{"correlation_id":"1","original_url":"https://practicum.yandex.ru"}]`),
	)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want %q", got, "gzip")
	}

	var resp []struct {
		CorrelationID string `json:"correlation_id"`
		ShortURL      string `json:"short_url"`
	}
	if err := json.Unmarshal(readGzip(t, rec.Body), &resp); err != nil {
		t.Fatalf("Unmarshal response: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("response len = %d, want %d", len(resp), 1)
	}
	if resp[0].CorrelationID != "1" {
		t.Fatalf("correlation_id = %q, want %q", resp[0].CorrelationID, "1")
	}
	if !strings.HasPrefix(resp[0].ShortURL, testBaseURL+"/") {
		t.Fatalf("short_url = %q, want prefix %q", resp[0].ShortURL, testBaseURL+"/")
	}
}

func TestShortenBatchWithGzipRequest(t *testing.T) {
	h := newTestServer(t)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/shorten/batch",
		gzipBody(t, `[{"correlation_id":"1","original_url":"https://practicum.yandex.ru"}]`),
	)
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

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
	if resp[0].CorrelationID != "1" {
		t.Fatalf("correlation_id = %q, want %q", resp[0].CorrelationID, "1")
	}
	if !strings.HasPrefix(resp[0].ShortURL, testBaseURL+"/") {
		t.Fatalf("short_url = %q, want prefix %q", resp[0].ShortURL, testBaseURL+"/")
	}
}

func TestBadGzipRequest(t *testing.T) {
	h := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/shorten", strings.NewReader("not gzip"))
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestPingWithoutDatabase(t *testing.T) {
	h := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestPingWithDatabase(t *testing.T) {
	h := newTestServerWithDB(t, openPingDB(t, false))
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestPingWithDatabaseError(t *testing.T) {
	h := newTestServerWithDB(t, openPingDB(t, true))
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestPingBadMethod(t *testing.T) {
	h := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/ping", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestPlainTextResponseIsNotGzipped(t *testing.T) {
	h := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("https://practicum.yandex.ru/"))
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
	if got := rec.Body.String(); !strings.HasPrefix(got, testBaseURL+"/") {
		t.Fatalf("body = %q, want prefix %q", got, testBaseURL+"/")
	}
}

func TestUserURLsNoContentAndCookie(t *testing.T) {
	h := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/user/urls", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if cookie := responseCookie(rec.Result()); cookie == nil {
		t.Fatal("auth cookie was not set")
	}
}

func TestUserURLsAfterPlainTextShorten(t *testing.T) {
	h := newTestServer(t)
	createReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("https://practicum.yandex.ru/"))
	createRec := httptest.NewRecorder()

	h.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}
	cookie := responseCookie(createRec.Result())
	if cookie == nil {
		t.Fatal("auth cookie was not set")
	}
	shortURL := createRec.Body.String()

	listReq := httptest.NewRequest(http.MethodGet, "/api/user/urls", nil)
	listReq.AddCookie(cookie)
	listReq.Header.Set("Accept-Encoding", "gzip")
	listRec := httptest.NewRecorder()

	h.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
	}
	if got := listRec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want %q", got, "gzip")
	}

	var resp []struct {
		ShortURL    string `json:"short_url"`
		OriginalURL string `json:"original_url"`
	}
	if err := json.Unmarshal(readGzip(t, listRec.Body), &resp); err != nil {
		t.Fatalf("Unmarshal response: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("response len = %d, want %d", len(resp), 1)
	}
	if resp[0].ShortURL != shortURL {
		t.Fatalf("short_url = %q, want %q", resp[0].ShortURL, shortURL)
	}
	if resp[0].OriginalURL != "https://practicum.yandex.ru/" {
		t.Fatalf("original_url = %q", resp[0].OriginalURL)
	}
}

func TestUserURLsAfterJSONAndBatchShorten(t *testing.T) {
	h := newTestServer(t)
	createReq := httptest.NewRequest(http.MethodPost, "/api/shorten", strings.NewReader(`{"url":"https://practicum.yandex.ru"}`))
	createRec := httptest.NewRecorder()

	h.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}
	cookie := responseCookie(createRec.Result())
	if cookie == nil {
		t.Fatal("auth cookie was not set")
	}

	batchReq := httptest.NewRequest(
		http.MethodPost,
		"/api/shorten/batch",
		strings.NewReader(`[{"correlation_id":"1","original_url":"https://example.com"}]`),
	)
	batchReq.AddCookie(cookie)
	batchRec := httptest.NewRecorder()

	h.ServeHTTP(batchRec, batchReq)

	if batchRec.Code != http.StatusCreated {
		t.Fatalf("batch status = %d, want %d", batchRec.Code, http.StatusCreated)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/user/urls", nil)
	listReq.AddCookie(cookie)
	listRec := httptest.NewRecorder()

	h.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
	}

	var resp []struct {
		ShortURL    string `json:"short_url"`
		OriginalURL string `json:"original_url"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("response len = %d, want %d", len(resp), 2)
	}
	if resp[0].OriginalURL != "https://practicum.yandex.ru" || resp[1].OriginalURL != "https://example.com" {
		t.Fatalf("response = %+v", resp)
	}
}

func TestUserURLsDoNotIncludeOtherUsersURLs(t *testing.T) {
	h := newTestServer(t)
	createReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("https://practicum.yandex.ru/"))
	createRec := httptest.NewRecorder()

	h.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/user/urls", nil)
	listRec := httptest.NewRecorder()

	h.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusNoContent {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusNoContent)
	}
}

func TestUserURLsSignedEmptyCookie(t *testing.T) {
	h := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/user/urls", nil)
	req.AddCookie(auth.NewCookie(""))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestUserURLsBadMethod(t *testing.T) {
	h := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/user/urls", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteUserURLs(t *testing.T) {
	h := newTestServer(t)
	createReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("https://practicum.yandex.ru/"))
	createRec := httptest.NewRecorder()

	h.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}
	cookie := responseCookie(createRec.Result())
	if cookie == nil {
		t.Fatal("auth cookie was not set")
	}
	shortURL := createRec.Body.String()
	id := strings.TrimPrefix(shortURL, testBaseURL+"/")

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/user/urls", strings.NewReader(`["`+id+`"]`))
	deleteReq.AddCookie(cookie)
	deleteRec := httptest.NewRecorder()

	h.ServeHTTP(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusAccepted {
		t.Fatalf("delete status = %d, want %d", deleteRec.Code, http.StatusAccepted)
	}

	waitForStatus(t, h, httptest.NewRequest(http.MethodGet, "/"+id, nil), http.StatusGone)

	listReq := httptest.NewRequest(http.MethodGet, "/api/user/urls", nil)
	listReq.AddCookie(cookie)
	waitForStatus(t, h, listReq, http.StatusNoContent)
}

func TestDeleteUserURLsBadRequest(t *testing.T) {
	h := newTestServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/user/urls", strings.NewReader(`{`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteUserURLsWithGzipRequest(t *testing.T) {
	h := newTestServer(t)
	createReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("https://practicum.yandex.ru/"))
	createRec := httptest.NewRecorder()

	h.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}
	cookie := responseCookie(createRec.Result())
	if cookie == nil {
		t.Fatal("auth cookie was not set")
	}
	id := strings.TrimPrefix(createRec.Body.String(), testBaseURL+"/")

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/user/urls", gzipBody(t, `["`+id+`"]`))
	deleteReq.Header.Set("Content-Encoding", "gzip")
	deleteReq.AddCookie(cookie)
	deleteRec := httptest.NewRecorder()

	h.ServeHTTP(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusAccepted {
		t.Fatalf("delete status = %d, want %d", deleteRec.Code, http.StatusAccepted)
	}

	waitForStatus(t, h, httptest.NewRequest(http.MethodGet, "/"+id, nil), http.StatusGone)
}

func newTestServer(t *testing.T) http.Handler {
	t.Helper()

	return newTestServerWithDB(t, nil)
}

func newTestServerWithDB(t *testing.T, db *sql.DB) http.Handler {
	t.Helper()

	h, err := server.New(config.Config{
		BaseURL:            testBaseURL,
		ServerAddress:      "localhost:8080",
		FileStoragePath:    t.TempDir() + "/storage.json",
		FileStoragePathSet: true,
	}, zap.NewNop(), db)
	if err != nil {
		t.Fatalf("server.New returned error: %v", err)
	}

	return h
}

func gzipBody(t *testing.T, body string) io.Reader {
	t.Helper()

	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write([]byte(body)); err != nil {
		t.Fatalf("gzip Write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("gzip Close: %v", err)
	}

	return &buf
}

func readGzip(t *testing.T, body io.Reader) []byte {
	t.Helper()

	reader, err := gzip.NewReader(body)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	return data
}

func responseCookie(resp *http.Response) *http.Cookie {
	for _, cookie := range resp.Cookies() {
		if cookie.Name == auth.CookieName {
			return cookie
		}
	}

	return nil
}

func waitForStatus(t *testing.T, h http.Handler, req *http.Request, want int) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req.Clone(req.Context()))
		if rec.Code == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("status = %d, want %d", rec.Code, want)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func openPingDB(t *testing.T, fail bool) *sql.DB {
	t.Helper()

	name := "ok"
	if fail {
		name = "fail"
	}

	db, err := sql.Open(pingDriverName, name)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	return db
}

type pingDriver struct{}

func (pingDriver) Open(name string) (driver.Conn, error) {
	conn := pingConn{}
	if name == "fail" {
		conn.err = errors.New("ping failed")
	}

	return conn, nil
}

type pingConn struct {
	err error
}

func (c pingConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not implemented")
}

func (c pingConn) Close() error {
	return nil
}

func (c pingConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not implemented")
}

func (c pingConn) Ping(context.Context) error {
	return c.err
}
