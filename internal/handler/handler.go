package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"github.com/go-chi/chi/v5"

	"github.com/true18/shortener/internal/repository"
)

type Handler struct {
	repo    repository.Store
	baseURL string
	router  chi.Router
}

func New(repo repository.Store, baseURL string) *Handler {
	h := &Handler{
		repo:    repo,
		baseURL: strings.TrimRight(baseURL, "/"),
	}

	r := chi.NewRouter()
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		badRequest(w)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		badRequest(w)
	})
	r.Post("/", h.create)
	r.Post("/api/shorten", h.createJSON)
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

	shortURL, err := h.shorten(originalURL)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusCreated)
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

	shortURL, err := h.shorten(originalURL)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(struct {
		Result string `json:"result"`
	}{
		Result: shortURL,
	})
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
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Location", originalURL)
	w.WriteHeader(http.StatusTemporaryRedirect)
}

func (h *Handler) shorten(originalURL string) (string, error) {
	id, err := h.repo.Save(originalURL)
	if err != nil {
		return "", err
	}

	return h.baseURL + "/" + id, nil
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
