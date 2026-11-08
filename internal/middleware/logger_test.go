package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLogResponseWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	w := newLogResponseWriter(rec)

	w.WriteHeader(http.StatusCreated)
	if _, err := w.Write([]byte("short")); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	if w.status != http.StatusCreated {
		t.Fatalf("status = %d, want %d", w.status, http.StatusCreated)
	}
	if w.size != 5 {
		t.Fatalf("size = %d, want %d", w.size, 5)
	}
}

func TestLogResponseWriterDefaultStatus(t *testing.T) {
	w := newLogResponseWriter(httptest.NewRecorder())

	if w.status != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.status, http.StatusOK)
	}
}

func TestLogResponseWriterWriteSetsOK(t *testing.T) {
	w := newLogResponseWriter(httptest.NewRecorder())

	if _, err := w.Write([]byte("ok")); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	if w.status != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.status, http.StatusOK)
	}
	if w.size != 2 {
		t.Fatalf("size = %d, want %d", w.size, 2)
	}
}
