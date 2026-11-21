package handler

import (
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
	repo    repository.URLStorer
	baseURL string
	router  chi.Router
}

func New(repo repository.URLStorer, baseURL string) *Handler {
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
