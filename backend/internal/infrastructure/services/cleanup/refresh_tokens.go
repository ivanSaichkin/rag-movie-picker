// Package cleanup contains background maintenance tasks.
package cleanup

import (
	"context"
	"log/slog"
	"time"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/ports"
)

const (
	defaultInterval  = 15 * time.Minute
	defaultRetention = 7 * 24 * time.Hour
)

// RefreshTokenCleaner removes expired and long-revoked refresh tokens. IsRevoked tokens
// are kept for the retention window, because reuse detection relies on them.
type RefreshTokenCleaner struct {
	repo      ports.RefreshTokenRepository
	logger    *slog.Logger
	interval  time.Duration
	retention time.Duration
}

// NewRefreshTokenCleaner builds the cleanup task.
func NewRefreshTokenCleaner(repo ports.RefreshTokenRepository, logger *slog.Logger) *RefreshTokenCleaner {
	return &RefreshTokenCleaner{
		repo:      repo,
		logger:    logger,
		interval:  defaultInterval,
		retention: defaultRetention,
	}
}

// Run cleans up once at startup and then on every tick until ctx is cancelled.
func (c *RefreshTokenCleaner) Run(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	c.cleanup(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.cleanup(ctx)
		}
	}
}

func (c *RefreshTokenCleaner) cleanup(ctx context.Context) {
	removed, err := c.repo.DeleteExpired(ctx, time.Now().Add(-c.retention))
	if err != nil {
		c.logger.Warn("refresh token cleanup failed", "error", err)
		return
	}
	if removed > 0 {
		c.logger.Info("stale refresh tokens removed", "count", removed)
	}
}
