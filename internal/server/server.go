package server

import (
	"net/http"

	"github.com/true18/shortener/internal/handler"
	"github.com/true18/shortener/internal/repository"
)

const (
	Addr    = ":8080"
	BaseURL = "http://localhost:8080"
)

func New() http.Handler {
	return handler.New(repository.NewMemory(), BaseURL)
}

func Run() error {
	return http.ListenAndServe(Addr, New())
}
