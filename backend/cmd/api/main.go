package main

import (
	"log"

	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/server"
)

func main() {
	cfg := config.Load()
	srv, err := server.New(cfg)
	if err != nil {
		log.Fatalf("server init: %v", err)
	}
	log.Printf("VPlayer API listening on %s", cfg.HTTPAddr)
	if err := srv.Run(cfg.HTTPAddr); err != nil {
		log.Fatalf("server run: %v", err)
	}
}
