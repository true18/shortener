package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"github.com/go-chi/chi/v5"

	"github.com/true18/shortener/internal/auth"
	"github.com/true18/shortener/internal/repository"
)

type Handler struct {
	repo    repository.Store
	baseURL string
	pinger  Pinger
	deletes DeleteEnqueuer
	router  chi.Router
}

type Pinger interface {
	PingContext(ctx context.Context) error
}

type DeleteEnqueuer interface {
	EnqueueDelete(userID string, ids []string) error
}

func New(repo repository.Store, baseURL string, pinger Pinger, deletes DeleteEnqueuer) *Handler {
	h := &Handler{
		repo:    repo,
		baseURL: strings.TrimRight(baseURL, "/"),
		pinger:  pinger,
		deletes: deletes,
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
	r.Get("/{id}", h.redirect)

	h.router = r

	return h
}

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

	originalURL := strings.TrimSpace(string(body))
	if !validURL(originalURL) {
		badRequest(w)
		return
	}

	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	shortURL, exists, err := h.shorten(originalURL, userID)
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

	originalURL := strings.TrimSpace(req.URL)
	if !validURL(originalURL) {
		badRequest(w)
		return
	}

	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	shortURL, exists, err := h.shorten(originalURL, userID)
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

		originalURL := strings.TrimSpace(item.OriginalURL)
		if !validURL(originalURL) {
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
		resp[i].ShortURL = h.shortURL(item.ShortID)
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

	originalURL, err := h.repo.Find(id)
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

	urls, err := h.repo.FindByUserID(userID)
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
		resp[i].ShortURL = h.shortURL(item.ShortID)
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
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			badRequest(w)
			return
		}
	}

	if h.deletes == nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	deleteIDs := append([]string(nil), ids...)
	if err := h.deletes.EnqueueDelete(userID, deleteIDs); err != nil {
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

func (h *Handler) shorten(originalURL string, userID string) (string, bool, error) {
	id, err := h.repo.Save(originalURL, userID)
	if errors.Is(err, repository.ErrURLExists) {
		return h.shortURL(id), true, nil
	}
	if err != nil {
		return "", false, err
	}

	return h.shortURL(id), false, nil
}

func (h *Handler) shortURL(id string) string {
	return h.baseURL + "/" + id
}

func validURL(rawURL string) bool {
	if strings.IndexFunc(rawURL, unicode.IsSpace) >= 0 {
		return false
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	return u.Hostname() != "" && (u.Scheme == "http" || u.Scheme == "https")
}

func badRequest(w http.ResponseWriter) {
	http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
}

func unauthorized(w http.ResponseWriter) {
	w.WriteHeader(http.StatusUnauthorized)
}
