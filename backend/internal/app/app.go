// Package app wires the service dependencies and runs the HTTP server.
package app

import (
	"context"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/config"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/adapters/router"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/persistence/postgres"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/services/cleanup"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/handlers"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server"
)

// App is the composed API service.
type App struct {
	pool    *pgxpool.Pool
	server  *server.Server
	cleaner *cleanup.RefreshTokens
}

// New builds the service and its dependencies. It opens the database pool and
// fails if the database is unreachable, so that a misconfigured process does
// not report itself as running.
func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*App, error) {
	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL, cfg.PGMaxConns)
	if err != nil {
		return nil, err
	}

	readiness, err := postgres.NewReadiness(pool)
	if err != nil {
		pool.Close()
		return nil, err
	}

	health := handlers.NewHealth(readiness)
	engine := router.New(logger, health)

	return &App{
		pool:    pool,
		server:  server.New(cfg.HTTPAddr, engine, logger),
		cleaner: cleanup.NewRefreshTokens(postgres.NewRefreshTokens(pool), logger),
	}, nil
}

// Run serves requests and runs background maintenance until ctx is cancelled.
func (a *App) Run(ctx context.Context) error {
	defer a.pool.Close()

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
