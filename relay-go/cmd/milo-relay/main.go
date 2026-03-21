package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"xmilo/relay-go/internal/app"
	"xmilo/relay-go/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	svc, err := app.New(cfg)
	if err != nil {
		log.Fatalf("bootstrap relay: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := svc.Start(ctx); err != nil {
		log.Fatalf("start relay: %v", err)
	}
}
