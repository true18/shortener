package service

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode"

	"go.uber.org/zap"

	"github.com/true18/shortener/internal/audit"
	"github.com/true18/shortener/internal/repository"
)

var (
	// ErrInvalidURL indicates that a URL is not a valid HTTP or HTTPS destination.
	ErrInvalidURL = errors.New("invalid URL")
	// ErrInvalidID indicates that a short identifier is empty or malformed.
	ErrInvalidID = errors.New("invalid short ID")
	// ErrUnauthorized indicates that a user-scoped operation has no user ID.
	ErrUnauthorized = errors.New("user is not authenticated")
)

// Shortener provides URL operations shared by HTTP and gRPC transports.
type Shortener struct {
	store   repository.Store
	baseURL string
	auditor audit.Observer
	logger  *zap.Logger
}

// NewShortener creates the shared URL application service.
func NewShortener(store repository.Store, baseURL string, auditor audit.Observer, logger *zap.Logger) *Shortener {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Shortener{
		store:   store,
		baseURL: strings.TrimRight(baseURL, "/"),
		auditor: auditor,
		logger:  logger,
	}
}

// NormalizeURL trims and validates an HTTP or HTTPS URL.
func NormalizeURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if strings.IndexFunc(rawURL, unicode.IsSpace) >= 0 {
		return "", ErrInvalidURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", ErrInvalidURL
	}

	return rawURL, nil
}

// Shorten validates and stores a URL, returning its full short URL and whether it already existed.
func (s *Shortener) Shorten(ctx context.Context, rawURL string, userID string) (string, bool, error) {
	originalURL, err := NormalizeURL(rawURL)
	if err != nil {
		return "", false, err
	}
	if userID == "" {
		return "", false, ErrUnauthorized
	}

	id, err := s.store.Save(originalURL, userID)
	if errors.Is(err, repository.ErrURLExists) {
		return s.ShortURL(id), true, nil
	}
	if err != nil {
		return "", false, err
	}

	s.audit(ctx, audit.ActionShorten, userID, originalURL)
	return s.ShortURL(id), false, nil
}

// Expand resolves a short ID and records a successful follow event.
func (s *Shortener) Expand(ctx context.Context, rawID string, userID string) (string, error) {
	id := strings.TrimSpace(rawID)
	if id == "" || strings.Contains(id, "/") || strings.IndexFunc(id, unicode.IsSpace) >= 0 {
		return "", ErrInvalidID
	}

	originalURL, err := s.store.Find(id)
	if err != nil {
		return "", err
	}

	s.audit(ctx, audit.ActionFollow, userID, originalURL)
	return originalURL, nil
}

// ListUserURLs returns active URLs owned by userID.
func (s *Shortener) ListUserURLs(userID string) ([]repository.UserURL, error) {
	if userID == "" {
		return nil, ErrUnauthorized
	}
	return s.store.FindByUserID(userID)
}

// ShortURL builds the public short URL for an identifier.
func (s *Shortener) ShortURL(id string) string {
	return s.baseURL + "/" + id
}

func (s *Shortener) audit(ctx context.Context, action string, userID string, originalURL string) {
	if s.auditor == nil {
		return
	}

	if err := s.auditor.Notify(ctx, audit.Event{
		Timestamp: time.Now().Unix(),
		Action:    action,
		UserID:    userID,
		URL:       originalURL,
	}); err != nil {
		s.logger.Error("audit delivery failed", zap.Error(err))
	}
}
