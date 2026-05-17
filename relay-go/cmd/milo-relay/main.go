package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"xmilo/relay-go/internal/config"
	httpx "xmilo/relay-go/internal/http"
)

const shutdownTimeout = 15 * time.Second

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load relay config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := httpx.NewApp(ctx, cfg)
	if err != nil {
		log.Fatalf("bootstrap relay: %v", err)
	}

	go func() {
		<-ctx.Done()
		time.Sleep(shutdownTimeout)
		log.Fatalf("relay shutdown exceeded timeout")
	}()

	log.Printf("milo relay starting addr=%s public_base_url=%s", cfg.HTTPAddr, cfg.PublicBaseURL)
	if err := app.Start(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("start relay: %v", err)
	}
}
