// Package grpcserver implements the gRPC transport for the URL shortener.
package grpcserver

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/true18/shortener/internal/auth"
	shortenerpb "github.com/true18/shortener/internal/proto"
	"github.com/true18/shortener/internal/repository"
	"github.com/true18/shortener/internal/service"
)

// Server implements the generated ShortenerService contract.
type Server struct {
	shortenerpb.UnimplementedShortenerServiceServer
	app *service.Shortener
}

// New creates and registers a gRPC server using the shared URL application service.
func New(app *service.Shortener, options ...grpc.ServerOption) *grpc.Server {
	options = append([]grpc.ServerOption{grpc.UnaryInterceptor(AuthorizationInterceptor())}, options...)
	server := grpc.NewServer(options...)
	shortenerpb.RegisterShortenerServiceServer(server, &Server{app: app})
	return server
}

// AuthorizationInterceptor verifies the raw signed cookie value in authorization metadata.
func AuthorizationInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		_ = info
		values := metadata.ValueFromIncomingContext(ctx, "authorization")
		if len(values) != 1 {
			return nil, status.Error(codes.Unauthenticated, "valid authorization metadata is required")
		}

		token := strings.TrimSpace(values[0])
		userID, ok := auth.Parse(token)
		if !ok || userID == "" {
			return nil, status.Error(codes.Unauthenticated, "valid authorization metadata is required")
		}

		return handler(auth.WithUserID(ctx, userID), req)
	}
}

// ShortenURL stores a URL for the authenticated user.
func (s *Server) ShortenURL(ctx context.Context, req *shortenerpb.URLShortenRequest) (*shortenerpb.URLShortenResponse, error) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication is required")
	}

	result, _, err := s.app.Shorten(ctx, req.GetUrl(), userID)
	if err != nil {
		return nil, grpcError(err)
	}
	return &shortenerpb.URLShortenResponse{Result: result}, nil
}

// ExpandURL resolves a short ID for the authenticated user.
func (s *Server) ExpandURL(ctx context.Context, req *shortenerpb.URLExpandRequest) (*shortenerpb.URLExpandResponse, error) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication is required")
	}

	result, err := s.app.Expand(ctx, req.GetId(), userID)
	if err != nil {
		return nil, grpcError(err)
	}
	return &shortenerpb.URLExpandResponse{Result: result}, nil
}

// ListUserURLs returns active URLs owned by the authenticated user.
func (s *Server) ListUserURLs(ctx context.Context, _ *emptypb.Empty) (*shortenerpb.UserURLsResponse, error) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication is required")
	}

	urls, err := s.app.ListUserURLs(userID)
	if err != nil {
		return nil, grpcError(err)
	}

	response := &shortenerpb.UserURLsResponse{Url: make([]*shortenerpb.URLData, len(urls))}
	for i, item := range urls {
		response.Url[i] = &shortenerpb.URLData{
			ShortUrl:    s.app.ShortURL(item.ShortID),
			OriginalUrl: item.OriginalURL,
		}
	}
	return response, nil
}

func grpcError(err error) error {
	switch {
	case errors.Is(err, service.ErrUnauthorized):
		return status.Error(codes.Unauthenticated, "authentication is required")
	case errors.Is(err, service.ErrInvalidURL), errors.Is(err, service.ErrInvalidID):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, repository.ErrNotFound):
		return status.Error(codes.NotFound, "short URL not found")
	case errors.Is(err, repository.ErrDeleted):
		return status.Error(codes.NotFound, "short URL was deleted")
	default:
		return status.Error(codes.Internal, "internal server error")
	}
}
