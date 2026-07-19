package main

import (
	"log"
	"os"

	"github.com/true18/shortener/internal/config"
	"github.com/true18/shortener/internal/server"
)

func main() {
	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}

	if err := server.Run(cfg); err != nil {
		log.Fatal(err)
	}
}
