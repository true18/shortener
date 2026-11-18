// Package middleware contains HTTP handlers that add cross-cutting request behavior.
package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// Gzip decompresses gzip request bodies and compresses supported responses when requested.
func Gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(strings.TrimSpace(r.Header.Get("Content-Encoding")), "gzip") {
			reader, err := gzip.NewReader(r.Body)
			if err != nil {
				_ = r.Body.Close()
				http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
				return
			}
			r.Body = gzipBody{
				reader: reader,
				body:   r.Body,
			}
		}

		if !acceptsGzip(r.Header.Get("Accept-Encoding")) {
			next.ServeHTTP(w, r)
			return
		}

		gw := newGzipResponseWriter(w)
		defer gw.Close()

		next.ServeHTTP(gw, r)
	})
}

type gzipBody struct {
	reader io.ReadCloser
	body   io.Closer
}

// Read reads bytes from the decompressed request body.
func (b gzipBody) Read(p []byte) (int, error) {
	return b.reader.Read(p)
}

// Close closes the decompressor and the original request body.
func (b gzipBody) Close() error {
	readerErr := b.reader.Close()
	bodyErr := b.body.Close()
	if readerErr != nil {
		return readerErr
	}

	return bodyErr
}

type gzipResponseWriter struct {
	http.ResponseWriter
	writer      *gzip.Writer
	status      int
	wroteHeader bool
	sentHeader  bool
}

func newGzipResponseWriter(w http.ResponseWriter) *gzipResponseWriter {
	return &gzipResponseWriter{
		ResponseWriter: w,
		status:         http.StatusOK,
	}
}

// WriteHeader records the response status until the headers are sent.
func (w *gzipResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}

	w.status = status
	w.wroteHeader = true
}

// Write sends response data, compressing it when its content type is supported.
func (w *gzipResponseWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}

	if !canGzip(w.Header().Get("Content-Type")) {
		w.sendHeader()
		return w.ResponseWriter.Write(data)
	}

	w.startGzip()
	return w.writer.Write(data)
}

// Close finishes compression and sends any deferred response headers.
func (w *gzipResponseWriter) Close() error {
	if w.writer != nil {
		return w.writer.Close()
	}
	if w.wroteHeader && !w.sentHeader {
		w.sendHeader()
	}

	return nil
}

func (w *gzipResponseWriter) startGzip() {
	if w.writer != nil {
		return
	}

	header := w.Header()
	header.Set("Content-Encoding", "gzip")
	header.Add("Vary", "Accept-Encoding")
	header.Del("Content-Length")

	w.writer = gzip.NewWriter(w.ResponseWriter)
	w.sendHeader()
}

func (w *gzipResponseWriter) sendHeader() {
	if w.sentHeader {
		return
	}

	w.sentHeader = true
	w.ResponseWriter.WriteHeader(w.status)
}

func acceptsGzip(value string) bool {
	gzipSeen := false
	gzipQuality := -1.0
	wildcardQuality := -1.0

	for _, item := range strings.Split(value, ",") {
		coding, quality, valid := parseEncoding(strings.TrimSpace(item))
		switch {
		case strings.EqualFold(coding, "gzip"):
			gzipSeen = true
			if valid && quality > gzipQuality {
				gzipQuality = quality
			}
		case coding == "*":
			if valid && quality > wildcardQuality {
				wildcardQuality = quality
			}
		}
	}

	if gzipSeen {
		return gzipQuality > 0
	}
	return wildcardQuality > 0
}

func parseEncoding(value string) (string, float64, bool) {
	parts := strings.Split(value, ";")
	coding := strings.TrimSpace(parts[0])
	if !validToken(coding) {
		return coding, 0, false
	}

	quality := 1.0
	qualitySet := false
	for _, parameter := range parts[1:] {
		name, rawValue, ok := strings.Cut(strings.TrimSpace(parameter), "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), "q") || qualitySet {
			return coding, 0, false
		}

		var valid bool
		quality, valid = parseQuality(strings.TrimSpace(rawValue))
		if !valid {
			return coding, 0, false
		}
		qualitySet = true
	}

	return coding, quality, true
}

func parseQuality(value string) (float64, bool) {
	if value == "0" || value == "1" {
		quality, _ := strconv.ParseFloat(value, 64)
		return quality, true
	}

	whole, fraction, ok := strings.Cut(value, ".")
	if !ok || len(fraction) > 3 {
		return 0, false
	}
	if whole != "0" && whole != "1" {
		return 0, false
	}
	if whole == "1" && strings.Trim(fraction, "0") != "" {
		return 0, false
	}
	for _, digit := range fraction {
		if digit < '0' || digit > '9' {
			return 0, false
		}
	}

	quality, err := strconv.ParseFloat(value, 64)
	return quality, err == nil
}

func validToken(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' {
			continue
		}
		switch char {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
			continue
		default:
			return false
		}
	}
	return true
}

func canGzip(contentType string) bool {
	contentType = strings.ToLower(contentType)

	return strings.HasPrefix(contentType, "application/json") ||
		strings.HasPrefix(contentType, "text/html")
}
