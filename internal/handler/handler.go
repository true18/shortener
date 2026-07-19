package handler

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"github.com/true18/shortener/internal/repository"
)

type Handler struct {
	repo    repository.URLRepository
	baseURL string
}

func New(repo repository.URLRepository, baseURL string) *Handler {
	return &Handler{
		repo:    repo,
		baseURL: strings.TrimRight(baseURL, "/"),
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.create(w, r)
	case http.MethodGet:
		h.redirect(w, r)
	default:
		badRequest(w)
	}
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		badRequest(w)
		return
	}

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

	id, err := h.repo.Save(originalURL)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(h.baseURL + "/" + id))
}

func (h *Handler) redirect(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/")
	if id == "" || strings.Contains(id, "/") {
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

func validURL(rawURL string) bool {
	if strings.IndexFunc(rawURL, unicode.IsSpace) >= 0 {
		return false
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	return u.IsAbs() && u.Hostname() != "" && (u.Scheme == "http" || u.Scheme == "https")
}

func badRequest(w http.ResponseWriter) {
	http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
}
