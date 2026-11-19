package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/true18/shortener/internal/config"
	"github.com/true18/shortener/internal/service"
)

func TestRunHTTPServerWaitsForActiveRequest(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server, err := newHTTPServer(config.Config{ServerAddress: "127.0.0.1:0"}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	if err != nil {
		t.Fatalf("newHTTPServer: %v", err)
	}

	listener := listenLocal(t)
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- runHTTPServer(ctx, server, listener, false, time.Second)
	}()

	requestDone := make(chan error, 1)
	go func() {
		resp, err := localHTTPClient(nil).Get("http://" + listener.Addr().String())
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusNoContent {
				err = errors.New("unexpected response status")
			}
		}
		requestDone <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	waitForListenerToClose(t, listener.Addr().String())
	select {
	case err := <-runDone:
		t.Fatalf("server stopped before active request completed: %v", err)
	default:
	}

	close(release)
	if err := <-requestDone; err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if err := <-runDone; err != nil {
		t.Fatalf("runHTTPServer returned error: %v", err)
	}
}

func TestRunHTTPServerGracefullyStopsHTTPAndHTTPS(t *testing.T) {
	for _, enableHTTPS := range []bool{false, true} {
		name := "HTTP"
		if enableHTTPS {
			name = "HTTPS"
		}
		t.Run(name, func(t *testing.T) {
			server, err := newHTTPServer(config.Config{
				ServerAddress: "127.0.0.1:0",
				EnableHTTPS:   enableHTTPS,
			}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			if err != nil {
				t.Fatalf("newHTTPServer: %v", err)
			}

			listener := listenLocal(t)
			ctx, cancel := context.WithCancel(context.Background())
			runDone := make(chan error, 1)
			go func() {
				runDone <- runHTTPServer(ctx, server, listener, enableHTTPS, time.Second)
			}()

			scheme := "http"
			var tlsConfig *tls.Config
			if enableHTTPS {
				scheme = "https"
				roots := x509.NewCertPool()
				roots.AddCert(server.TLSConfig.Certificates[0].Leaf)
				tlsConfig = &tls.Config{
					MinVersion: tls.VersionTLS12,
					RootCAs:    roots,
				}
			}
			resp, err := localHTTPClient(tlsConfig).Get(scheme + "://" + listener.Addr().String())
			if err != nil {
				t.Fatalf("GET: %v", err)
			}
			if err := resp.Body.Close(); err != nil {
				t.Fatalf("Close response: %v", err)
			}

			cancel()
			if err := <-runDone; err != nil {
				t.Fatalf("runHTTPServer returned error: %v", err)
			}
		})
	}
}

func TestRunHTTPServerReportsShutdownTimeout(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })

	server, err := newHTTPServer(config.Config{ServerAddress: "127.0.0.1:0"}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(started)
		<-release
	}))
	if err != nil {
		t.Fatalf("newHTTPServer: %v", err)
	}
	listener := listenLocal(t)
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- runHTTPServer(ctx, server, listener, false, 20*time.Millisecond)
	}()
	requestDone := make(chan struct{})
	go func() {
		resp, getErr := localHTTPClient(nil).Get("http://" + listener.Addr().String())
		if getErr == nil {
			_ = resp.Body.Close()
		}
		close(requestDone)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	err = <-runDone
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("runHTTPServer error = %v, want deadline exceeded", err)
	}

	releaseOnce.Do(func() { close(release) })
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("request did not finish after forced close")
	}
}

func TestManagedHandlerCloseWaitsForDeletesAndIsIdempotent(t *testing.T) {
	store := &orderedDeleteStore{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	deletes := service.NewDeleteQueue(store)
	closer := &countingCloser{}
	handler := &managedHandler{deletes: deletes, closers: []io.Closer{closer}}
	if err := deletes.EnqueueDelete("user", []string{"id"}); err != nil {
		t.Fatalf("EnqueueDelete: %v", err)
	}

	closed := make(chan error, 1)
	go func() { closed <- handler.Close() }()
	select {
	case <-store.started:
	case <-time.After(time.Second):
		t.Fatal("delete did not start")
	}
	if closer.calls.Load() != 0 {
		t.Fatal("dependent closer ran before delete completed")
	}
	close(store.release)
	if err := <-closed; err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := handler.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if closer.calls.Load() != 1 {
		t.Fatalf("closer calls = %d, want 1", closer.calls.Load())
	}
}

func localHTTPClient(tlsConfig *tls.Config) *http.Client {
	return &http.Client{
		Timeout: time.Second,
		Transport: &http.Transport{
			Proxy:           nil,
			TLSClientConfig: tlsConfig,
		},
	}
}

func waitForListenerToClose(t *testing.T, address string) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for {
		connection, err := net.DialTimeout("tcp", address, 10*time.Millisecond)
		if err != nil {
			return
		}
		_ = connection.Close()
		if time.Now().After(deadline) {
			t.Fatal("listener still accepts connections")
		}
		time.Sleep(time.Millisecond)
	}
}

type orderedDeleteStore struct {
	started chan struct{}
	release chan struct{}
}

func (s *orderedDeleteStore) DeleteUserURLs([]string, string) error {
	close(s.started)
	<-s.release
	return nil
}

type countingCloser struct {
	calls atomic.Int32
}

func (c *countingCloser) Close() error {
	c.calls.Add(1)
	return nil
}
