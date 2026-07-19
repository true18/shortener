package main

import (
	"log"

	"github.com/true18/shortener/internal/server"
)

func main() {
	if err := server.Run(); err != nil {
		log.Fatal(err)
	}
}
