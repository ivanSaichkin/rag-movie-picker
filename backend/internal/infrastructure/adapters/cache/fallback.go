package cache

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/ports"
)

const fallbackWarnInterval = time.Minute

// FallbackCache prefers the primary cache and uses the secondary one when the
// primary fails, so that an unavailable Redis degrades the service instead of
// breaking it.
type FallbackCache struct {
	primary   ports.Cache
	secondary ports.Cache
	logger    *slog.Logger
	now       func() time.Time

	mu       sync.Mutex
	lastWarn time.Time
}

// NewFallbackCache builds the degrading cache.
func NewFallbackCache(primary, secondary ports.Cache, logger *slog.Logger) *FallbackCache {
	return &FallbackCache{
		primary:   primary,
		secondary: secondary,
		logger:    logger,
		now:       time.Now,
	}
}

// Get reads from the primary cache and falls back on errors.
func (f *FallbackCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	value, found, err := f.primary.Get(ctx, key)
	if err == nil {
		return value, found, nil
	}
	f.warn("cache get failed", err)
	return f.secondary.Get(ctx, key)
}

// Set writes to the primary cache and falls back on errors.
func (f *FallbackCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := f.primary.Set(ctx, key, value, ttl); err != nil {
		f.warn("cache set failed", err)
		return f.secondary.Set(ctx, key, value, ttl)
	}
	return nil
}

// Increment increments the counter in the primary cache and falls back on errors.
func (f *FallbackCache) Increment(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	value, err := f.primary.Increment(ctx, key, ttl)
	if err == nil {
		return value, nil
	}
	f.warn("cache incr failed", err)
	return f.secondary.Increment(ctx, key, ttl)
}

// Delete removes the key from the primary cache and falls back on errors.
func (f *FallbackCache) Delete(ctx context.Context, key string) error {
	if err := f.primary.Delete(ctx, key); err != nil {
		f.warn("cache delete failed", err)
		return f.secondary.Delete(ctx, key)
	}
	return nil
}

// warn logs at most once per interval: a broken Redis would otherwise flood the
// log on every request.
func (f *FallbackCache) warn(message string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := f.now()
	if !f.lastWarn.IsZero() && now.Sub(f.lastWarn) < fallbackWarnInterval {
		return
	}
	f.lastWarn = now
	f.logger.Warn(message, "error", err)
}
