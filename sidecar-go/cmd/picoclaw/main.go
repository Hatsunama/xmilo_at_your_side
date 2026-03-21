package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"xmilo/sidecar-go/internal/app"
	"xmilo/sidecar-go/internal/config"
)

func main() {
	configPath := flag.String("config", "", "path to config.json (overrides PICOCLAW_CONFIG env var)")
	flag.Parse()

	// --config flag takes priority over env var
	if *configPath != "" {
		os.Setenv("PICOCLAW_CONFIG", *configPath)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	svc, err := app.New(cfg)
	if err != nil {
		log.Fatalf("bootstrap sidecar: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := svc.Start(ctx); err != nil {
		log.Fatalf("start sidecar: %v", err)
	}
}
