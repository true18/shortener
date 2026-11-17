package server

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/true18/shortener/internal/config"
	"go.uber.org/zap"
)

func TestHTTPServerServesHTTP(t *testing.T) {
	server, err := newHTTPServer(config.Config{ServerAddress: "127.0.0.1:0"}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	if err != nil {
		t.Fatalf("newHTTPServer: %v", err)
	}

	listener := listenLocal(t)
	done := make(chan error, 1)
	go func() {
		done <- server.Serve(listener)
	}()

	resp, err := http.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	shutdownServer(t, server, done)
}

func TestHTTPServerServesHTTPS(t *testing.T) {
	server, err := newHTTPServer(config.Config{
		ServerAddress: "127.0.0.1:0",
		EnableHTTPS:   true,
	}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "secure")
	}))
	if err != nil {
		t.Fatalf("newHTTPServer: %v", err)
	}

	roots := x509.NewCertPool()
	roots.AddCert(server.TLSConfig.Certificates[0].Leaf)
	client := &http.Client{
		Timeout:   2 * time.Second,
		Transport: &http.Transport{TLSClientConfig: server.TLSConfig.Clone()},
	}
	client.Transport.(*http.Transport).TLSClientConfig.RootCAs = roots
	client.Transport.(*http.Transport).TLSClientConfig.Certificates = nil

	listener := listenLocal(t)
	done := make(chan error, 1)
	go func() {
		done <- server.ServeTLS(listener, "", "")
	}()

	resp, err := client.Get("https://" + listener.Addr().String())
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(body) != "secure" {
		t.Fatalf("body = %q, want %q", body, "secure")
	}

	shutdownServer(t, server, done)
}

func TestNewHTTPServerReportsCertificateError(t *testing.T) {
	_, err := newTLSConfig("localhost:8080", failingReader{})
	if err == nil {
		t.Fatal("newTLSConfig returned nil error")
	}
}

func TestRunReportsListenError(t *testing.T) {
	err := Run(config.Config{
		ServerAddress: "bad address",
		BaseURL:       config.DefaultBaseURL,
		EnableHTTPS:   true,
	}, zap.NewNop(), nil)
	if err == nil {
		t.Fatal("Run returned nil error")
	}
}

func listenLocal(t *testing.T) net.Listener {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})

	return listener
}

func shutdownServer(t *testing.T, server *http.Server, done <-chan error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := <-done; !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("Serve returned %v, want http.ErrServerClosed", err)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("random source failed")
}

func TestCertificateIncludesConfiguredHost(t *testing.T) {
	tlsConfig, err := newTLSConfig("example.test:8443", strings.NewReader(strings.Repeat("a", 512)))
	if err != nil {
		t.Fatalf("newTLSConfig: %v", err)
	}
	if err := tlsConfig.Certificates[0].Leaf.VerifyHostname("example.test"); err != nil {
		t.Fatalf("VerifyHostname: %v", err)
	}
}
