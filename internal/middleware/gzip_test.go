package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGzipTextHTML(t *testing.T) {
	h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<h1>ok</h1>"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want %q", got, "gzip")
	}

	body := readGzip(t, rec.Body)
	if body != "<h1>ok</h1>" {
		t.Fatalf("body = %q, want %q", body, "<h1>ok</h1>")
	}
}

func TestAcceptsGzip(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{name: "empty", want: false},
		{name: "gzip", header: "gzip", want: true},
		{name: "multiple first", header: "gzip, deflate", want: true},
		{name: "multiple last", header: "br, gzip", want: true},
		{name: "case insensitive", header: "GZip", want: true},
		{name: "quality one", header: "gzip;q=1.0", want: true},
		{name: "quality fraction", header: "br, gzip;q=0.5", want: true},
		{name: "quality zero", header: "gzip;q=0", want: false},
		{name: "wildcard", header: "*;q=1", want: true},
		{name: "wildcard zero", header: "*;q=0", want: false},
		{name: "explicit gzip rejects wildcard", header: "gzip;q=0, *;q=1", want: false},
		{name: "explicit gzip accepts over wildcard", header: "gzip;q=0.5, *;q=0", want: true},
		{name: "duplicate gzip uses highest quality", header: "gzip;q=0, gzip;q=0.5", want: true},
		{name: "other encoding", header: "br, deflate", want: false},
		{name: "substring is not coding", header: "xgzip", want: false},
		{name: "invalid quality", header: "gzip;q=invalid", want: false},
		{name: "quality above one", header: "gzip;q=1.1", want: false},
		{name: "too many decimals", header: "gzip;q=0.1234", want: false},
		{name: "missing leading zero", header: "gzip;q=.5", want: false},
		{name: "unknown parameter", header: "gzip;level=1", want: false},
		{name: "explicit malformed gzip rejects wildcard", header: "gzip;q=bad, *;q=1", want: false},
		{name: "empty list items", header: ", br, , gzip ,", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := acceptsGzip(tt.header); got != tt.want {
				t.Fatalf("acceptsGzip(%q) = %t, want %t", tt.header, got, tt.want)
			}
		})
	}
}

func TestGzipQualityZeroDisablesCompression(t *testing.T) {
	h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip;q=0, *;q=1")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"ok":true}` {
		t.Fatalf("body = %q", got)
	}
}

func readGzip(t *testing.T, body io.Reader) string {
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

	return strings.TrimSpace(string(data))
}
