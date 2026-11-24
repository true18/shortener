// Package handler implements the HTTP API for shortening and resolving URLs.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/true18/shortener/internal/audit"
	"github.com/true18/shortener/internal/auth"
	"github.com/true18/shortener/internal/repository"
	"github.com/true18/shortener/internal/service"
)

// Handler routes URL API requests to a repository and its supporting services.
type Handler struct {
	repo    repository.Store
	pinger  Pinger
	deletes DeleteEnqueuer
	app     *service.Shortener
	trusted *net.IPNet
	router  chi.Router
}

// Pinger checks the health of the database used by the server.
type Pinger interface {
	// PingContext reports whether the database is reachable within ctx.
	PingContext(ctx context.Context) error
}

// DeleteEnqueuer accepts user-scoped URL deletion tasks for background processing.
type DeleteEnqueuer interface {
	// EnqueueDelete queues IDs for soft deletion by userID.
	EnqueueDelete(userID string, ids []string) error
}

// New creates an HTTP handler without audit event delivery.
func New(repo repository.Store, baseURL string, pinger Pinger, deletes DeleteEnqueuer) *Handler {
	return NewWithAudit(repo, baseURL, pinger, deletes, nil, nil)
}

// NewWithAudit creates a handler that publishes successful shorten and follow events to auditor.
func NewWithAudit(
	repo repository.Store,
	baseURL string,
	pinger Pinger,
	deletes DeleteEnqueuer,
	auditor audit.Observer,
	logger *zap.Logger,
) *Handler {
	return NewWithAuditAndTrustedSubnet(repo, baseURL, pinger, deletes, auditor, logger, nil)
}

// NewWithAuditAndTrustedSubnet creates a handler with audit delivery and protected internal statistics.
func NewWithAuditAndTrustedSubnet(
	repo repository.Store,
	baseURL string,
	pinger Pinger,
	deletes DeleteEnqueuer,
	auditor audit.Observer,
	logger *zap.Logger,
	trustedSubnet *net.IPNet,
) *Handler {
	app := service.NewShortener(repo, baseURL, auditor, logger)
	return NewWithApplication(repo, pinger, deletes, app, trustedSubnet)
}

// NewWithApplication creates an HTTP handler using the shared URL application service.
func NewWithApplication(
	repo repository.Store,
	pinger Pinger,
	deletes DeleteEnqueuer,
	app *service.Shortener,
	trustedSubnet *net.IPNet,
) *Handler {
	h := &Handler{
		repo:    repo,
		pinger:  pinger,
		deletes: deletes,
		app:     app,
		trusted: trustedSubnet,
	}

	r := chi.NewRouter()
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		badRequest(w)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		badRequest(w)
	})
	r.Post("/", h.create)
	r.Post("/api/shorten/batch", h.createBatch)
	r.Post("/api/shorten", h.createJSON)
	r.Get("/api/user/urls", h.userURLs)
	r.Delete("/api/user/urls", h.deleteUserURLs)
	r.Get("/ping", h.ping)
	r.Get("/api/internal/stats", h.stats)
	r.Get("/{id}", h.redirect)

	h.router = r

	return h
}

func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	if !isTrustedIP(h.trusted, r.Header.Get("X-Real-IP")) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	stats, err := h.repo.Stats()
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(stats)
}

func isTrustedIP(network *net.IPNet, value string) bool {
	if network == nil {
		return false
	}

	ip := net.ParseIP(strings.TrimSpace(value))
	return ip != nil && network.Contains(ip)
}

// ServeHTTP dispatches a request to the matching URL API endpoint.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.router.ServeHTTP(w, r)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		badRequest(w)
		return
	}
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		badRequest(w)
		return
	}

	userID, _ := auth.UserIDFromContext(r.Context())
	shortURL, exists, err := h.app.Shorten(r.Context(), string(body), userID)
	if errors.Is(err, service.ErrInvalidURL) {
		badRequest(w)
		return
	}
	if errors.Is(err, service.ErrUnauthorized) {
		unauthorized(w)
		return
	}
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	if exists {
		w.WriteHeader(http.StatusConflict)
	} else {
		w.WriteHeader(http.StatusCreated)
	}
	_, _ = w.Write([]byte(shortURL))
}

func (h *Handler) createJSON(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/shorten" {
		badRequest(w)
		return
	}
	defer r.Body.Close()

	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w)
		return
	}

	userID, _ := auth.UserIDFromContext(r.Context())
	shortURL, exists, err := h.app.Shorten(r.Context(), req.URL, userID)
	if errors.Is(err, service.ErrInvalidURL) {
		badRequest(w)
		return
	}
	if errors.Is(err, service.ErrUnauthorized) {
		unauthorized(w)
		return
	}
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if exists {
		w.WriteHeader(http.StatusConflict)
	} else {
		w.WriteHeader(http.StatusCreated)
	}
	_ = json.NewEncoder(w).Encode(struct {
		Result string `json:"result"`
	}{
		Result: shortURL,
	})
}

func (h *Handler) createBatch(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/shorten/batch" {
		badRequest(w)
		return
	}
	defer r.Body.Close()

	var req []struct {
		CorrelationID string `json:"correlation_id"`
		OriginalURL   string `json:"original_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w)
		return
	}
	if len(req) == 0 {
		badRequest(w)
		return
	}

	items := make([]repository.BatchItem, len(req))
	for i, item := range req {
		if strings.TrimSpace(item.CorrelationID) == "" {
			badRequest(w)
			return
		}

		originalURL, err := service.NormalizeURL(item.OriginalURL)
		if err != nil {
			badRequest(w)
			return
		}

		items[i] = repository.BatchItem{
			CorrelationID: item.CorrelationID,
			OriginalURL:   originalURL,
		}
	}

	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	results, err := h.repo.SaveBatch(items, userID)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	resp := make([]struct {
		CorrelationID string `json:"correlation_id"`
		ShortURL      string `json:"short_url"`
	}, len(results))
	for i, item := range results {
		resp[i].CorrelationID = item.CorrelationID
		resp[i].ShortURL = h.app.ShortURL(item.ShortID)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) redirect(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		badRequest(w)
		return
	}

	userID, _ := auth.UserIDFromContext(r.Context())
	originalURL, err := h.app.Expand(r.Context(), id, userID)
	if errors.Is(err, service.ErrInvalidID) {
		badRequest(w)
		return
	}
	if errors.Is(err, repository.ErrNotFound) {
		badRequest(w)
		return
	}
	if errors.Is(err, repository.ErrDeleted) {
		w.WriteHeader(http.StatusGone)
		return
	}
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Location", originalURL)
	w.WriteHeader(http.StatusTemporaryRedirect)
}

func (h *Handler) userURLs(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/user/urls" {
		badRequest(w)
		return
	}

	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	urls, err := h.app.ListUserURLs(userID)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if len(urls) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	resp := make([]struct {
		ShortURL    string `json:"short_url"`
		OriginalURL string `json:"original_url"`
	}, len(urls))
	for i, item := range urls {
		resp[i].ShortURL = h.app.ShortURL(item.ShortID)
		resp[i].OriginalURL = item.OriginalURL
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) deleteUserURLs(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/user/urls" {
		badRequest(w)
		return
	}
	defer r.Body.Close()

	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	var ids []string
	if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
		badRequest(w)
		return
	}
	if len(ids) == 0 {
		badRequest(w)
		return
	}
	trimmedIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			badRequest(w)
			return
		}
		trimmedIDs = append(trimmedIDs, id)
	}

	if h.deletes == nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	if err := h.deletes.EnqueueDelete(userID, trimmedIDs); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) ping(w http.ResponseWriter, r *http.Request) {
	if h.pinger == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := h.pinger.PingContext(r.Context()); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func badRequest(w http.ResponseWriter) {
	http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
}

func unauthorized(w http.ResponseWriter) {
	w.WriteHeader(http.StatusUnauthorized)
}
