// Package app wires the service dependencies and runs the HTTP server.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/config"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/ports"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/adapters/cache"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/adapters/password"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/adapters/router"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/adapters/token"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/persistence/postgres"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/services/auth"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/services/cleanup"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/handlers"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server/middleware"
)

// App is the composed API service.
type App struct {
	logger     *slog.Logger
	pool       *pgxpool.Pool
	server     *server.Server
	cleaner    *cleanup.RefreshTokens
	closeCache func() error
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

	users := postgres.NewUsers(pool)
	tokens := postgres.NewRefreshTokens(pool)

	hasher := password.New(cfg.Argon2.Iterations, cfg.Argon2.MemoryMiB, cfg.Argon2.Parallelism)
	issuer := token.New(cfg.JWTSecret, cfg.AccessTokenTTL)

	authService, err := auth.New(users, tokens, hasher, issuer, cfg.RefreshTokenTTL)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("build auth service: %w", err)
	}

	store, closeStore := buildCache(cfg, logger)
	authHandler := handlers.NewAuth(authService, cfg.CookieSecure)

	health := handlers.NewHealth(readiness)
	engine := router.New(router.Params{
		Logger:        logger,
		Health:        health,
		Auth:          authHandler,
		Authenticate:  middleware.Authenticate(issuer),
		AllowedOrigin: cfg.AllowedOrigin,
		Limiters: router.Limiters{
			Login:    middleware.RateLimit(store, "login", cfg.RateLimit.AuthPerMinute, time.Minute),
			Register: middleware.RateLimit(store, "register", cfg.RateLimit.RegisterPerHour, time.Hour),
			Refresh:  middleware.RateLimit(store, "refresh", cfg.RateLimit.RefreshPerMinute, time.Minute),
		},
	})

	return &App{
		logger:     logger,
		pool:       pool,
		server:     server.New(cfg.HTTPAddr, engine, logger),
		cleaner:    cleanup.NewRefreshTokens(tokens, logger),
		closeCache: closeStore,
	}, nil
}

// Run serves requests and runs background maintenance until ctx is cancelled.
func (a *App) Run(ctx context.Context) error {
	defer a.pool.Close()
	defer func() {
		if err := a.closeCache(); err != nil {
			a.logger.Warn("cache close failed", "error", err)
		}
	}()

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

// buildCache selects Redis when it is configured and wraps it with the
// in-memory fallback, so that Redis is a soft dependency.
func buildCache(cfg config.Config, logger *slog.Logger) (ports.Cache, func() error) {
	memory := cache.NewMemory()
	if cfg.Redis.Addr == "" {
		logger.Info("cache: in-memory")
		return memory, func() error { return nil }
	}

	redisCache := cache.NewRedis(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB, cfg.Redis.Timeout)
	logger.Info("cache: redis with in-memory fallback", "addr", cfg.Redis.Addr)
	return cache.NewFallback(redisCache, memory, logger), redisCache.Close
}
