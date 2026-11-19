package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/true18/shortener/internal/auth"
	"github.com/true18/shortener/internal/handler"
	"github.com/true18/shortener/internal/repository"
)

const exampleUserID = "user-123"

func ExampleNew_plainText() {
	store := repository.NewMemory()
	h := handler.New(store, "http://localhost:8080", nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("https://example.com/article"))
	req = req.WithContext(auth.WithUserID(req.Context(), exampleUserID))
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)

	fmt.Println(response.Code == http.StatusCreated)
	fmt.Println(response.Header().Get("Content-Type"))
	fmt.Println(strings.HasPrefix(response.Body.String(), "http://localhost:8080/"))
	// Output:
	// true
	// text/plain
	// true
}

func ExampleNew_json() {
	store := repository.NewMemory()
	h := handler.New(store, "http://localhost:8080", nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/shorten", strings.NewReader(`{"url":"https://example.com/article"}`))
	req = req.WithContext(auth.WithUserID(req.Context(), exampleUserID))
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)

	var result struct {
		URL string `json:"result"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		panic(err)
	}
	fmt.Println(response.Code)
	fmt.Println(response.Header().Get("Content-Type"))
	fmt.Println(strings.HasPrefix(result.URL, "http://localhost:8080/"))
	// Output:
	// 201
	// application/json
	// true
}

func ExampleNew_batch() {
	store := repository.NewMemory()
	h := handler.New(store, "http://localhost:8080", nil, nil)
	items := []struct {
		CorrelationID string `json:"correlation_id"`
		OriginalURL   string `json:"original_url"`
	}{
		{CorrelationID: "first", OriginalURL: "https://example.com/first"},
		{CorrelationID: "second", OriginalURL: "https://example.com/second"},
	}
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(items); err != nil {
		panic(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/shorten/batch", &body)
	req = req.WithContext(auth.WithUserID(req.Context(), exampleUserID))
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)

	var results []struct {
		CorrelationID string `json:"correlation_id"`
		ShortURL      string `json:"short_url"`
	}
	if err := json.NewDecoder(response.Body).Decode(&results); err != nil {
		panic(err)
	}
	fmt.Println(response.Code)
	fmt.Println(len(results))
	fmt.Println(results[0].CorrelationID, results[1].CorrelationID)
	fmt.Println(strings.HasPrefix(results[0].ShortURL, "http://localhost:8080/"))
	// Output:
	// 201
	// 2
	// first second
	// true
}

func ExampleNew_redirect() {
	store := repository.NewMemory()
	id, err := store.Save("https://example.com/article", exampleUserID)
	if err != nil {
		panic(err)
	}
	h := handler.New(store, "http://localhost:8080", nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/"+id, nil)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)

	fmt.Println(response.Code)
	fmt.Println(response.Header().Get("Location"))
	// Output:
	// 307
	// https://example.com/article
}

func ExampleNew_userURLs() {
	store := repository.NewMemory()
	if _, err := store.Save("https://example.com/article", exampleUserID); err != nil {
		panic(err)
	}
	h := handler.New(store, "http://localhost:8080", nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/user/urls", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), exampleUserID))
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)

	var urls []struct {
		OriginalURL string `json:"original_url"`
	}
	if err := json.NewDecoder(response.Body).Decode(&urls); err != nil {
		panic(err)
	}
	fmt.Println(response.Code)
	fmt.Println(urls[0].OriginalURL)
	// Output:
	// 200
	// https://example.com/article
}

func ExampleNew_ping() {
	h := handler.New(repository.NewMemory(), "http://localhost:8080", examplePinger{}, nil)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ping", nil))

	fmt.Println(response.Code)
	// Output: 200
}

type examplePinger struct{}

// PingContext implements handler.Pinger for the ping endpoint example.
func (examplePinger) PingContext(context.Context) error { return nil }
