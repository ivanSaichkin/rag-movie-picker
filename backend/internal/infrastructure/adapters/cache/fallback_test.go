package cache_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/adapters/cache"
)

// failingCache always fails, as an unreachable Redis does.
type failingCache struct{}

var errCacheDown = errors.New("cache is down")

func (failingCache) Get(context.Context, string) ([]byte, bool, error) {
	return nil, false, errCacheDown
}

func (failingCache) Set(context.Context, string, []byte, time.Duration) error {
	return errCacheDown
}

func (failingCache) Increment(context.Context, string, time.Duration) (int64, error) {
	return 0, errCacheDown
}

func (failingCache) Delete(context.Context, string) error {
	return errCacheDown
}

func TestFallbackUsesSecondaryWhenPrimaryFails(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))

	memory := cache.NewInMemoryCache()
	store := cache.NewFallbackCache(failingCache{}, memory, logger)
	ctx := context.Background()

	if err := store.Set(ctx, "key", []byte("value"), time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	value, found, err := store.Get(ctx, "key")
	if err != nil || !found || string(value) != "value" {
		t.Fatalf("Get() = %q, %t, %v", value, found, err)
	}

	first, err := store.Increment(ctx, "counter", time.Minute)
	if err != nil || first != 1 {
		t.Fatalf("Increment() = %d, %v", first, err)
	}
	second, err := store.Increment(ctx, "counter", time.Minute)
	if err != nil || second != 2 {
		t.Fatalf("Increment() = %d, %v", second, err)
	}

	if err := store.Delete(ctx, "key"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Every call failed, but the warning is throttled to one record per
	// interval: a broken Redis must not flood the log.
	if rate := strings.Count(logs.String(), `"level":"WARN"`); rate != 1 {
		t.Fatalf("warning records = %d, want 1; logs = %s", rate, logs.String())
	}
	if !strings.Contains(logs.String(), errCacheDown.Error()) {
		t.Fatalf("cause must be logged: %s", logs.String())
	}
}

func TestFallbackPrefersPrimary(t *testing.T) {
	primary := cache.NewInMemoryCache()
	secondary := cache.NewInMemoryCache()
	store := cache.NewFallbackCache(primary, secondary, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	ctx := context.Background()

	if err := store.Set(ctx, "key", []byte("primary-value"), time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// The secondary must stay untouched while the primary works.
	if _, found, _ := secondary.Get(ctx, "key"); found {
		t.Fatal("secondary cache must not be written while the primary works")
	}
	value, found, err := store.Get(ctx, "key")
	if err != nil || !found || string(value) != "primary-value" {
		t.Fatalf("Get() = %q, %t, %v", value, found, err)
	}
}
