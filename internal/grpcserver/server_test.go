package grpcserver_test

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/true18/shortener/internal/audit"
	"github.com/true18/shortener/internal/auth"
	"github.com/true18/shortener/internal/grpcserver"
	shortenerpb "github.com/true18/shortener/internal/proto"
	"github.com/true18/shortener/internal/repository"
	"github.com/true18/shortener/internal/service"
)

const grpcTestBaseURL = "http://short.test"

func TestShortenExpandAndListUserURLs(t *testing.T) {
	store := repository.NewMemory()
	observer := &recordingObserver{}
	client := newTestClient(t, service.NewShortener(store, grpcTestBaseURL, observer, zap.NewNop()))
	userOne := authorizedContext(t, "user-one")
	userTwo := authorizedContext(t, "user-two")

	shortened, err := client.ShortenURL(userOne, &shortenerpb.URLShortenRequest{Url: " https://example.com/long "})
	if err != nil {
		t.Fatalf("ShortenURL: %v", err)
	}
	if !strings.HasPrefix(shortened.GetResult(), grpcTestBaseURL+"/") {
		t.Fatalf("result = %q", shortened.GetResult())
	}

	duplicate, err := client.ShortenURL(userTwo, &shortenerpb.URLShortenRequest{Url: "https://example.com/long"})
	if err != nil {
		t.Fatalf("duplicate ShortenURL: %v", err)
	}
	if duplicate.GetResult() != shortened.GetResult() {
		t.Fatalf("duplicate result = %q, want %q", duplicate.GetResult(), shortened.GetResult())
	}

	id := strings.TrimPrefix(shortened.GetResult(), grpcTestBaseURL+"/")
	expanded, err := client.ExpandURL(userOne, &shortenerpb.URLExpandRequest{Id: id})
	if err != nil {
		t.Fatalf("ExpandURL: %v", err)
	}
	if expanded.GetResult() != "https://example.com/long" {
		t.Fatalf("expanded result = %q", expanded.GetResult())
	}

	listOne, err := client.ListUserURLs(userOne, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("ListUserURLs user one: %v", err)
	}
	if len(listOne.GetUrl()) != 1 || listOne.GetUrl()[0].GetShortUrl() != shortened.GetResult() || listOne.GetUrl()[0].GetOriginalUrl() != "https://example.com/long" {
		t.Fatalf("user one URLs = %+v", listOne.GetUrl())
	}

	listTwo, err := client.ListUserURLs(userTwo, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("ListUserURLs user two: %v", err)
	}
	if len(listTwo.GetUrl()) != 0 {
		t.Fatalf("user two URLs = %+v, want empty", listTwo.GetUrl())
	}

	events := observer.Events()
	if len(events) != 2 || events[0].Action != audit.ActionShorten || events[1].Action != audit.ActionFollow {
		t.Fatalf("audit events = %+v", events)
	}
}

func TestAuthenticationAndValidationErrors(t *testing.T) {
	client := newTestClient(t, service.NewShortener(repository.NewMemory(), grpcTestBaseURL, nil, zap.NewNop()))

	tests := []struct {
		name string
		ctx  context.Context
		url  string
		code codes.Code
	}{
		{name: "missing metadata", ctx: context.Background(), url: "https://example.com", code: codes.Unauthenticated},
		{name: "bad signature", ctx: outgoingAuthorization("invalid"), url: "https://example.com", code: codes.Unauthenticated},
		{name: "Bearer format is not accepted", ctx: outgoingAuthorization("Bearer " + auth.NewCookie("user").Value), url: "https://example.com", code: codes.Unauthenticated},
		{name: "signed empty user", ctx: outgoingAuthorization(auth.NewCookie("").Value), url: "https://example.com", code: codes.Unauthenticated},
		{name: "invalid URL", ctx: authorizedContext(t, "user"), url: "not-a-url", code: codes.InvalidArgument},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.ShortenURL(tt.ctx, &shortenerpb.URLShortenRequest{Url: tt.url})
			if status.Code(err) != tt.code {
				t.Fatalf("code = %v, want %v, error = %v", status.Code(err), tt.code, err)
			}
		})
	}

	if _, err := client.ListUserURLs(context.Background(), &emptypb.Empty{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("ListUserURLs without metadata code = %v", status.Code(err))
	}
	if _, err := client.ListUserURLs(outgoingAuthorization("invalid"), &emptypb.Empty{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("ListUserURLs with invalid token code = %v", status.Code(err))
	}
}

func TestExpandErrors(t *testing.T) {
	store := repository.NewMemory()
	client := newTestClient(t, service.NewShortener(store, grpcTestBaseURL, nil, zap.NewNop()))
	ctx := authorizedContext(t, "user")

	if _, err := client.ExpandURL(ctx, &shortenerpb.URLExpandRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("empty ID code = %v, error = %v", status.Code(err), err)
	}
	if _, err := client.ExpandURL(ctx, &shortenerpb.URLExpandRequest{Id: "unknown"}); status.Code(err) != codes.NotFound {
		t.Fatalf("unknown ID code = %v, error = %v", status.Code(err), err)
	}

	shortened, err := client.ShortenURL(ctx, &shortenerpb.URLShortenRequest{Url: "https://example.com/deleted"})
	if err != nil {
		t.Fatalf("ShortenURL: %v", err)
	}
	id := strings.TrimPrefix(shortened.GetResult(), grpcTestBaseURL+"/")
	if err := store.DeleteUserURLs([]string{id}, "user"); err != nil {
		t.Fatalf("DeleteUserURLs: %v", err)
	}
	if _, err := client.ExpandURL(ctx, &shortenerpb.URLExpandRequest{Id: id}); status.Code(err) != codes.NotFound {
		t.Fatalf("deleted ID code = %v, error = %v", status.Code(err), err)
	}
}

func TestRepositoryErrorsAreInternal(t *testing.T) {
	client := newTestClient(t, service.NewShortener(errorStore{}, grpcTestBaseURL, nil, zap.NewNop()))
	ctx := authorizedContext(t, "user")

	if _, err := client.ShortenURL(ctx, &shortenerpb.URLShortenRequest{Url: "https://example.com"}); status.Code(err) != codes.Internal {
		t.Fatalf("ShortenURL code = %v", status.Code(err))
	}
	if _, err := client.ExpandURL(ctx, &shortenerpb.URLExpandRequest{Id: "abc12345"}); status.Code(err) != codes.Internal {
		t.Fatalf("ExpandURL code = %v", status.Code(err))
	}
	if _, err := client.ListUserURLs(ctx, &emptypb.Empty{}); status.Code(err) != codes.Internal {
		t.Fatalf("ListUserURLs code = %v", status.Code(err))
	}
}

func TestAuthorizationInterceptorStoresUserID(t *testing.T) {
	interceptor := grpcserver.AuthorizationInterceptor()
	token := auth.NewCookie("user-42").Value
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", token))

	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, _ any) (any, error) {
		userID, ok := auth.UserIDFromContext(ctx)
		if !ok || userID != "user-42" {
			t.Fatalf("user ID = %q, ok = %t", userID, ok)
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
}

func newTestClient(t *testing.T, app *service.Shortener) shortenerpb.ShortenerServiceClient {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := grpcserver.New(app)
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()

	connection, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		server.Stop()
		t.Fatalf("NewClient: %v", err)
	}

	t.Cleanup(func() {
		_ = connection.Close()
		server.Stop()
		_ = listener.Close()
		if err := <-serveDone; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("Serve: %v", err)
		}
	})
	return shortenerpb.NewShortenerServiceClient(connection)
}

func authorizedContext(t *testing.T, userID string) context.Context {
	t.Helper()
	return outgoingAuthorization(auth.NewCookie(userID).Value)
}

func outgoingAuthorization(value string) context.Context {
	return metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", value))
}

type recordingObserver struct {
	mu     sync.Mutex
	events []audit.Event
}

func (o *recordingObserver) Notify(_ context.Context, event audit.Event) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, event)
	return nil
}

func (o *recordingObserver) Events() []audit.Event {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]audit.Event(nil), o.events...)
}

type errorStore struct{}

func (errorStore) Save(string, string) (string, error) {
	return "", errors.New("save failed")
}

func (errorStore) SaveBatch([]repository.BatchItem, string) ([]repository.BatchResult, error) {
	return nil, errors.New("save batch failed")
}

func (errorStore) Find(string) (string, error) {
	return "", errors.New("find failed")
}

func (errorStore) FindByUserID(string) ([]repository.UserURL, error) {
	return nil, errors.New("list failed")
}

func (errorStore) DeleteUserURLs([]string, string) error {
	return errors.New("delete failed")
}

func (errorStore) Stats() (repository.Stats, error) {
	return repository.Stats{}, errors.New("stats failed")
}
