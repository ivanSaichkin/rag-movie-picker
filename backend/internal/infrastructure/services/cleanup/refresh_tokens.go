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

// RefreshTokens removes expired and long-revoked refresh tokens. Revoked tokens
// are kept for the retention window, because reuse detection relies on them.
type RefreshTokens struct {
	repo      ports.RefreshTokenRepository
	logger    *slog.Logger
	interval  time.Duration
	retention time.Duration
}

// NewRefreshTokens builds the cleanup task.
func NewRefreshTokens(repo ports.RefreshTokenRepository, logger *slog.Logger) *RefreshTokens {
	return &RefreshTokens{
		repo:      repo,
		logger:    logger,
		interval:  defaultInterval,
		retention: defaultRetention,
	}
}

// Run cleans up once at startup and then on every tick until ctx is cancelled.
func (c *RefreshTokens) Run(ctx context.Context) {
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

func (c *RefreshTokens) cleanup(ctx context.Context) {
	removed, err := c.repo.DeleteStale(ctx, time.Now().Add(-c.retention))
	if err != nil {
		c.logger.Warn("refresh token cleanup failed", "error", err)
		return
	}
	if removed > 0 {
		c.logger.Info("stale refresh tokens removed", "count", removed)
	}
}
