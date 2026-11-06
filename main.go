package main

import (
	_ "embed"
	"log"

	"github.com/taurusxin/ghproxy-go/internal/config"
	"github.com/taurusxin/ghproxy-go/internal/server"
)

//go:embed templates/index.html
var indexHTML string

func main() {
	cfg := config.FromEnv()
	router := server.New(indexHTML)
	log.Printf("GitHub Proxy is running on http://%s", cfg.Address())
	if err := router.Run(cfg.Address()); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
