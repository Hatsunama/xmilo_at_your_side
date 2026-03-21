package app

import (
	"context"

	"xmilo/relay-go/internal/config"
	httpx "xmilo/relay-go/internal/http"
)

type Service struct {
	app *httpx.App
}

func New(cfg config.Config) (*Service, error) {
	a, err := httpx.NewApp(context.Background(), cfg)
	if err != nil {
		return nil, err
	}
	return &Service{app: a}, nil
}

func (s *Service) Start(ctx context.Context) error {
	return s.app.Start(ctx)
}
