package main

import (
	"crypto/rand"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const (
	addr    = ":8080"
	baseURL = "http://localhost:8080/"
	idChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	idLen   = 8
)

type server struct {
	mu    sync.RWMutex
	links map[string]string
	ids   map[string]string
}

func main() {
	s := &server{
		links: make(map[string]string),
		ids:   make(map[string]string),
	}

	if err := http.ListenAndServe(addr, s); err != nil {
		log.Fatal(err)
	}
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.create(w, r)
	case http.MethodGet:
		s.redirect(w, r)
	default:
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
	}
}

func (s *server) create(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	rawURL := strings.TrimSpace(string(body))
	if !validURL(rawURL) {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	id, err := s.idFor(rawURL)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(baseURL + id))
}

func (s *server) redirect(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/")
	if id == "" || strings.Contains(id, "/") {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	rawURL, ok := s.links[id]
	s.mu.RUnlock()
	if !ok {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	w.Header().Set("Location", rawURL)
	w.WriteHeader(http.StatusTemporaryRedirect)
}

func (s *server) idFor(rawURL string) (string, error) {
	s.mu.RLock()
	id, ok := s.ids[rawURL]
	s.mu.RUnlock()
	if ok {
		return id, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if id, ok := s.ids[rawURL]; ok {
		return id, nil
	}

	for {
		id, err := newID()
		if err != nil {
			return "", err
		}
		if _, exists := s.links[id]; exists {
			continue
		}

		s.links[id] = rawURL
		s.ids[rawURL] = id
		return id, nil
	}
}

func validURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	return u.IsAbs() && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https")
}

func newID() (string, error) {
	id := make([]byte, idLen)
	max := big.NewInt(int64(len(idChars)))

	for i := range id {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		id[i] = idChars[n.Int64()]
	}

	return string(id), nil
}
