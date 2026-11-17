package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/true18/shortener/internal/auth"
	"github.com/true18/shortener/internal/config"
	"github.com/true18/shortener/internal/grpcserver"
	shortenerpb "github.com/true18/shortener/internal/proto"
	"github.com/true18/shortener/internal/repository"
	"github.com/true18/shortener/internal/service"
)

func TestRunHTTPAndGRPCServesAndStops(t *testing.T) {
	for _, enableHTTPS := range []bool{false, true} {
		name := "HTTP"
		if enableHTTPS {
			name = "HTTPS"
		}
		t.Run(name, func(t *testing.T) {
			cfg := config.Config{ServerAddress: "127.0.0.1:0", EnableHTTPS: enableHTTPS}
			httpServer, err := newHTTPServer(cfg, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			if err != nil {
				t.Fatalf("newHTTPServer: %v", err)
			}

			app := service.NewShortener(repository.NewMemory(), "https://short.test", nil, zap.NewNop())
			var serverOptions []grpc.ServerOption
			var clientCredentials credentials.TransportCredentials = insecure.NewCredentials()
			var clientTLS *tls.Config
			if enableHTTPS {
				serverOptions = append(serverOptions, grpc.Creds(credentials.NewTLS(httpServer.TLSConfig.Clone())))
				roots := x509.NewCertPool()
				roots.AddCert(httpServer.TLSConfig.Certificates[0].Leaf)
				clientTLS = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
				clientCredentials = credentials.NewTLS(clientTLS)
			}
			grpcServer := grpcserver.New(app, serverOptions...)
			httpListener := listenLocal(t)
			grpcListener := listenLocal(t)
			ctx, cancel := context.WithCancel(context.Background())
			runDone := make(chan error, 1)
			go func() {
				runDone <- runHTTPAndGRPC(ctx, httpServer, httpListener, enableHTTPS, grpcServer, grpcListener, time.Second)
			}()

			scheme := "http"
			if enableHTTPS {
				scheme = "https"
			}
			response, err := localHTTPClient(clientTLS).Get(scheme + "://" + httpListener.Addr().String())
			if err != nil {
				cancel()
				t.Fatalf("HTTP GET: %v", err)
			}
			_ = response.Body.Close()
			if response.StatusCode != http.StatusNoContent {
				t.Fatalf("HTTP status = %d", response.StatusCode)
			}

			connection, err := grpc.NewClient(
				"passthrough:///"+grpcListener.Addr().String(),
				grpc.WithTransportCredentials(clientCredentials),
			)
			if err != nil {
				cancel()
				t.Fatalf("grpc.NewClient: %v", err)
			}
			defer connection.Close()
			client := shortenerpb.NewShortenerServiceClient(connection)
			rpcCtx, rpcCancel := context.WithTimeout(authorizedOutgoingContext("user"), time.Second)
			defer rpcCancel()
			shortened, err := client.ShortenURL(rpcCtx, &shortenerpb.URLShortenRequest{Url: "https://example.com"})
			if err != nil {
				cancel()
				t.Fatalf("ShortenURL: %v", err)
			}
			if shortened.GetResult() == "" {
				t.Fatal("ShortenURL returned an empty result")
			}

			cancel()
			if err := <-runDone; err != nil {
				t.Fatalf("runHTTPAndGRPC: %v", err)
			}
		})
	}
}

func TestRunContextSharesStorageBetweenHTTPAndGRPC(t *testing.T) {
	httpAddress := reserveAddress(t)
	grpcAddress := reserveAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	stopped := false
	go func() {
		runDone <- RunContext(ctx, config.Config{
			ServerAddress: httpAddress,
			GRPCAddress:   grpcAddress,
			BaseURL:       "http://" + httpAddress,
		}, zap.NewNop(), nil)
	}()
	t.Cleanup(func() {
		if stopped {
			return
		}
		cancel()
		select {
		case <-runDone:
		case <-time.After(time.Second):
		}
	})

	waitForHTTPServer(t, httpAddress)
	request, err := http.NewRequest(http.MethodPost, "http://"+httpAddress+"/", strings.NewReader("https://example.com/shared"))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	request.Header.Set("Content-Type", "text/plain")
	response, err := localHTTPClient(nil).Do(request)
	if err != nil {
		t.Fatalf("HTTP shorten: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("HTTP status = %d", response.StatusCode)
	}
	var authCookieValue string
	for _, cookie := range response.Cookies() {
		if cookie.Name == auth.CookieName {
			authCookieValue = cookie.Value
		}
	}
	if authCookieValue == "" {
		t.Fatal("HTTP response did not contain an auth cookie")
	}

	connection, err := grpc.NewClient("passthrough:///"+grpcAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	defer connection.Close()
	rpcCtx, rpcCancel := context.WithTimeout(outgoingTokenContext(authCookieValue), time.Second)
	defer rpcCancel()
	urls, err := shortenerpb.NewShortenerServiceClient(connection).ListUserURLs(rpcCtx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("ListUserURLs: %v", err)
	}
	if len(urls.GetUrl()) != 1 || urls.GetUrl()[0].GetOriginalUrl() != "https://example.com/shared" {
		t.Fatalf("gRPC URLs = %+v", urls.GetUrl())
	}

	cancel()
	if err := <-runDone; err != nil {
		t.Fatalf("RunContext: %v", err)
	}
	stopped = true
}

func TestRunHTTPAndGRPCWaitsForActiveRPC(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	implementation := &blockingShortenerServer{started: started, release: release}
	grpcServer := grpc.NewServer()
	shortenerpb.RegisterShortenerServiceServer(grpcServer, implementation)

	httpServer, err := newHTTPServer(config.Config{ServerAddress: "127.0.0.1:0"}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	if err != nil {
		t.Fatalf("newHTTPServer: %v", err)
	}
	httpListener := listenLocal(t)
	grpcListener := listenLocal(t)
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- runHTTPAndGRPC(ctx, httpServer, httpListener, false, grpcServer, grpcListener, time.Second)
	}()

	connection, err := grpc.NewClient("passthrough:///"+grpcListener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	defer connection.Close()
	client := shortenerpb.NewShortenerServiceClient(connection)
	rpcDone := make(chan error, 1)
	go func() {
		_, err := client.ShortenURL(context.Background(), &shortenerpb.URLShortenRequest{Url: "https://example.com"})
		rpcDone <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("RPC did not start")
	}
	cancel()
	select {
	case err := <-runDone:
		t.Fatalf("servers stopped before the active RPC completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	close(release)
	if err := <-rpcDone; err != nil {
		t.Fatalf("ShortenURL: %v", err)
	}
	if err := <-runDone; err != nil {
		t.Fatalf("runHTTPAndGRPC: %v", err)
	}
}

func TestRunHTTPAndGRPCForcesStopAfterTimeout(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	implementation := &blockingShortenerServer{started: started, release: release}
	grpcServer := grpc.NewServer()
	shortenerpb.RegisterShortenerServiceServer(grpcServer, implementation)
	httpServer, err := newHTTPServer(config.Config{ServerAddress: "127.0.0.1:0"}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	if err != nil {
		t.Fatalf("newHTTPServer: %v", err)
	}
	httpListener := listenLocal(t)
	grpcListener := listenLocal(t)
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- runHTTPAndGRPC(ctx, httpServer, httpListener, false, grpcServer, grpcListener, 20*time.Millisecond)
	}()

	connection, err := grpc.NewClient("passthrough:///"+grpcListener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	defer connection.Close()
	rpcDone := make(chan error, 1)
	go func() {
		_, err := shortenerpb.NewShortenerServiceClient(connection).ShortenURL(context.Background(), &shortenerpb.URLShortenRequest{})
		rpcDone <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("RPC did not start")
	}
	cancel()
	err = <-runDone
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("runHTTPAndGRPC error = %v, want deadline exceeded", err)
	}
	close(release)
	select {
	case <-rpcDone:
	case <-time.After(time.Second):
		t.Fatal("RPC did not finish after forced stop")
	}
}

func TestRunHTTPAndGRPCStopsHTTPOnGRPCServeError(t *testing.T) {
	httpServer, err := newHTTPServer(config.Config{ServerAddress: "127.0.0.1:0"}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	if err != nil {
		t.Fatalf("newHTTPServer: %v", err)
	}
	httpListener := listenLocal(t)
	grpcListener := listenLocal(t)
	if err := grpcListener.Close(); err != nil {
		t.Fatalf("close gRPC listener: %v", err)
	}

	err = runHTTPAndGRPC(context.Background(), httpServer, httpListener, false, grpc.NewServer(), grpcListener, time.Second)
	if err == nil {
		t.Fatal("runHTTPAndGRPC returned nil error")
	}
	waitForListenerToClose(t, httpListener.Addr().String())
}

func TestRunContextReportsGRPCListenError(t *testing.T) {
	occupied := listenLocal(t)
	err := RunContext(context.Background(), config.Config{
		ServerAddress: "127.0.0.1:0",
		GRPCAddress:   occupied.Addr().String(),
		BaseURL:       "http://short.test",
	}, zap.NewNop(), nil)
	if err == nil {
		t.Fatal("RunContext returned nil error")
	}
}

func authorizedOutgoingContext(userID string) context.Context {
	return outgoingTokenContext(auth.NewCookie(userID).Value)
}

func outgoingTokenContext(token string) context.Context {
	return metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", token))
}

func reserveAddress(t *testing.T) string {
	t.Helper()
	listener := listenLocal(t)
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close reserved listener: %v", err)
	}
	return address
}

func waitForHTTPServer(t *testing.T, address string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		response, err := localHTTPClient(nil).Get("http://" + address + "/ping")
		if err == nil {
			_ = response.Body.Close()
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("HTTP server did not start")
}

type blockingShortenerServer struct {
	shortenerpb.UnimplementedShortenerServiceServer
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *blockingShortenerServer) ShortenURL(context.Context, *shortenerpb.URLShortenRequest) (*shortenerpb.URLShortenResponse, error) {
	s.once.Do(func() { close(s.started) })
	<-s.release
	return &shortenerpb.URLShortenResponse{Result: "http://short.test/id"}, nil
}
