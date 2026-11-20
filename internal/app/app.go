// Package app wires the service dependencies and runs the HTTP server.
package app

import (
	"context"
	"log/slog"

	"github.com/ivanSaichkin/rag-movie-picker/internal/config"
	"github.com/ivanSaichkin/rag-movie-picker/internal/infrastructure/adapters/router"
	"github.com/ivanSaichkin/rag-movie-picker/internal/interfaces/http/handlers"
	"github.com/ivanSaichkin/rag-movie-picker/internal/interfaces/http/server"
)

// App is the composed API service.
type App struct {
	server *server.Server
}

// New builds the service. Readiness checks are registered by the stages that
// add dependencies; PostgreSQL and RAG arrive in the next parts.
func New(cfg config.Config, logger *slog.Logger) *App {
	health := handlers.NewHealth()
	engine := router.New(logger, health)

	return &App{
		server: server.New(cfg.HTTPAddr, engine, logger),
	}
}

// Run serves requests until ctx is cancelled.
func (a *App) Run(ctx context.Context) error {
	return a.server.Run(ctx)
}
