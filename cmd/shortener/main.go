package main

import (
	"fmt"
	"log"
	"os"

	"github.com/true18/shortener/internal/config"
	"github.com/true18/shortener/internal/server"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatal(err)
	}
	defer logger.Sync()

	if err := server.Run(cfg, logger); err != nil {
		log.Fatal(err)
	}
}
