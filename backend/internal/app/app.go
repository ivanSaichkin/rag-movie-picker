// Package app wires the service dependencies and runs the HTTP server.
package app

import (
	"context"
	"log/slog"
	"sync"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/config"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/services/cleanup"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server"
)

// App is the composed API service.
type App struct {
	logger         *slog.Logger
	infrastructure *infrastructure
	server         *server.Server
	cleaner        *cleanup.RefreshTokenCleaner
}

// New builds the service through the factories in this package: resources,
// application services, then the HTTP layer.
func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*App, error) {
	infra, err := newInfrastructure(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}

	appServices, err := newServices(infra, cfg, logger)
	if err != nil {
		infra.Close(logger)
		return nil, err
	}

	return &App{
		logger:         logger,
		infrastructure: infra,
		server:         newHTTPServer(cfg, logger, infra, appServices),
		cleaner:        appServices.cleaner,
	}, nil
}

// Run serves requests and runs background maintenance until ctx is cancelled.
func (a *App) Run(ctx context.Context) error {
	defer a.infrastructure.Close(a.logger)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var group sync.WaitGroup
	group.Go(func() {
		a.cleaner.Run(ctx)
	})

	err := a.server.Run(ctx)
	cancel()
	group.Wait()

	return err
}
