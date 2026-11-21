package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/config"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/ports"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/adapters/cache"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/persistence/postgres"
)

// infrastructure holds the long-lived resources of the process: the PostgreSQL
// pool, the cache and the RAG connection.
type infrastructure struct {
	pool          *pgxpool.Pool
	cache         ports.Cache
	ragConnection *grpc.ClientConn

	closeCache func() error
	closeRAG   func() error
}

// newInfrastructure opens the resources and releases them itself when a later
// step fails, so a partially built service never leaks a connection.
func newInfrastructure(ctx context.Context, cfg config.Config, logger *slog.Logger) (*infrastructure, error) {
	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL, cfg.PostgresMaxConnections)
	if err != nil {
		return nil, err
	}

	store, closeStore := buildCache(cfg, logger)

	ragConnection, err := grpc.NewClient(cfg.RAGGRPCAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		if closeErr := closeStore(); closeErr != nil {
			logger.Warn("cache close failed", "error", closeErr)
		}
		pool.Close()
		return nil, fmt.Errorf("create rag connection: %w", err)
	}

	return &infrastructure{
		pool:          pool,
		cache:         store,
		ragConnection: ragConnection,
		closeCache:    closeStore,
		closeRAG:      ragConnection.Close,
	}, nil
}

// Close releases the resources in the reverse order of acquisition.
func (i *infrastructure) Close(logger *slog.Logger) {
	if err := i.closeRAG(); err != nil {
		logger.Warn("rag connection close failed", "error", err)
	}
	if err := i.closeCache(); err != nil {
		logger.Warn("cache close failed", "error", err)
	}
	i.pool.Close()
}

// buildCache selects Redis when it is configured and wraps it with the
// in-memory fallback, so that Redis is a soft dependency.
func buildCache(cfg config.Config, logger *slog.Logger) (ports.Cache, func() error) {
	memory := cache.NewInMemoryCache()
	if cfg.Redis.Address == "" {
		logger.Info("cache: in-memory")
		return memory, func() error { return nil }
	}

	redisCache := cache.NewRedisCache(cfg.Redis.Address, cfg.Redis.Password, cfg.Redis.Database, cfg.Redis.Timeout)
	logger.Info("cache: redis with in-memory fallback", "addr", cfg.Redis.Address)
	return cache.NewFallbackCache(redisCache, memory, logger), redisCache.Close
}
