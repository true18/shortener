package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

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

func (b gzipBody) Read(p []byte) (int, error) {
	return b.reader.Read(p)
}

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

func (w *gzipResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}

	w.status = status
	w.wroteHeader = true
}

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
	return strings.Contains(strings.ToLower(value), "gzip")
}

func canGzip(contentType string) bool {
	contentType = strings.ToLower(contentType)

	return strings.HasPrefix(contentType, "application/json") ||
		strings.HasPrefix(contentType, "text/html")
}
