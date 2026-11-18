package handler_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/true18/shortener/internal/auth"
	"github.com/true18/shortener/internal/handler"
	"github.com/true18/shortener/internal/middleware"
	"github.com/true18/shortener/internal/repository"
)

var benchmarkResponse *httptest.ResponseRecorder

const benchmarkUser = "benchmark-user"

func BenchmarkCreate(b *testing.B) {
	repo := repository.NewMemory()
	h := benchmarkHandler(repo)
	urls := benchmarkURLs(512)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(fmt.Sprintf("%s/%d", urls[i%len(urls)], i)))
		benchmarkResponse = httptest.NewRecorder()
		h.ServeHTTP(benchmarkResponse, req.WithContext(auth.WithUserID(req.Context(), benchmarkUser)))
	}
}

func BenchmarkCreateJSON(b *testing.B) {
	repo := repository.NewMemory()
	h := benchmarkHandler(repo)
	urls := benchmarkURLs(512)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body := fmt.Sprintf(`{"url":%q}`, fmt.Sprintf("%s/%d", urls[i%len(urls)], i))
		req := httptest.NewRequest(http.MethodPost, "/api/shorten", strings.NewReader(body))
		benchmarkResponse = httptest.NewRecorder()
		h.ServeHTTP(benchmarkResponse, req.WithContext(auth.WithUserID(req.Context(), benchmarkUser)))
	}
}

func BenchmarkCreateBatch(b *testing.B) {
	const batchSize = 32
	repo := repository.NewMemory()
	h := benchmarkHandler(repo)
	urls := benchmarkURLs(batchSize)
	var builder strings.Builder
	for i, rawURL := range urls {
		if i > 0 {
			builder.WriteByte(',')
		}
		fmt.Fprintf(&builder, `{"correlation_id":%q,"original_url":%q}`, fmt.Sprint(i), rawURL)
	}
	body := "[" + builder.String() + "]"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/shorten/batch", strings.NewReader(body))
		benchmarkResponse = httptest.NewRecorder()
		h.ServeHTTP(benchmarkResponse, req.WithContext(auth.WithUserID(req.Context(), benchmarkUser)))
	}
}

func BenchmarkRedirect(b *testing.B) {
	repo := repository.NewMemory()
	id, err := repo.Save("https://example.com/existing", benchmarkUser)
	if err != nil {
		b.Fatal(err)
	}
	h := benchmarkHandler(repo)
	req := httptest.NewRequest(http.MethodGet, "/"+id, nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkResponse = httptest.NewRecorder()
		h.ServeHTTP(benchmarkResponse, req)
	}
}

func BenchmarkGzipJSON(b *testing.B) {
	payload := `{"orders":[{"number":"9278923470","status":"PROCESSED","accrual":500,"uploaded_at":"2020-12-10T15:15:45+03:00"},{"number":"12345678903","status":"PROCESSING","uploaded_at":"2020-12-10T15:12:01+03:00"}],"balance":{"current":1250.5,"withdrawn":42}}`
	base := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	})
	h := middleware.Gzip(base)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/user/orders", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		benchmarkResponse = httptest.NewRecorder()
		h.ServeHTTP(benchmarkResponse, req)
	}
}

func benchmarkHandler(repo repository.Store) http.Handler {
	return handler.New(repo, "http://localhost:8080", nil, nil)
}

func benchmarkURLs(count int) []string {
	urls := make([]string, count)
	for i := range urls {
		switch i % 4 {
		case 0:
			urls[i] = fmt.Sprintf("https://ex.am/%d", i)
		case 1:
			urls[i] = fmt.Sprintf("https://shop.example.com/products/category/item-%d?campaign=spring&source=benchmark", i)
		case 2:
			urls[i] = fmt.Sprintf("https://example.org/search?q=product-%d&page=%d", i, i%17)
		default:
			urls[i] = fmt.Sprintf("https://example.net/a/long/path/to/resource/%d?lang=en&currency=USD&ref=bench", i)
		}
	}

	return urls
}
