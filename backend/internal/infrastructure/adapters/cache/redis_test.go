package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/adapters/cache"
)

func newRedis(t *testing.T) (*cache.RedisCache, *miniredis.Miniredis) {
	t.Helper()

	server := miniredis.RunT(t)
	store := cache.NewRedisCache(server.Addr(), "", 0, time.Second)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close redis: %v", err)
		}
	})
	return store, server
}

func TestRedisSetGetDelete(t *testing.T) {
	store, _ := newRedis(t)
	ctx := context.Background()

	if err := store.Set(ctx, "key", []byte("value"), time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	value, found, err := store.Get(ctx, "key")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !found || string(value) != "value" {
		t.Fatalf("Get() = %q, %t", value, found)
	}

	if err := store.Delete(ctx, "key"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, found, err := store.Get(ctx, "key"); err != nil || found {
		t.Fatalf("Get() after delete = %t, %v", found, err)
	}
}

func TestRedisExpiresValues(t *testing.T) {
	store, server := newRedis(t)
	ctx := context.Background()

	if err := store.Set(ctx, "key", []byte("value"), time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	server.FastForward(2 * time.Minute)

	if _, found, err := store.Get(ctx, "key"); err != nil || found {
		t.Fatalf("Get() after expiry = %t, %v", found, err)
	}
}

func TestRedisIncrSetsExpiryOnlyOnCreation(t *testing.T) {
	store, server := newRedis(t)
	ctx := context.Background()

	for want := int64(1); want <= 3; want++ {
		value, err := store.Increment(ctx, "counter", time.Minute)
		if err != nil {
			t.Fatalf("Increment() error = %v", err)
		}
		if value != want {
			t.Fatalf("Increment() = %d, want %d", value, want)
		}
	}

	if ttl := server.TTL("counter"); ttl <= 0 || ttl > time.Minute {
		t.Fatalf("TTL = %v, want a fixed minute window", ttl)
	}
}

func TestRedisIncrResetsAfterExpiry(t *testing.T) {
	store, server := newRedis(t)
	ctx := context.Background()

	if _, err := store.Increment(ctx, "counter", time.Minute); err != nil {
		t.Fatalf("Increment() error = %v", err)
	}
	server.FastForward(2 * time.Minute)

	value, err := store.Increment(ctx, "counter", time.Minute)
	if err != nil {
		t.Fatalf("Increment() error = %v", err)
	}
	if value != 1 {
		t.Fatalf("Increment() after window = %d, want 1", value)
	}
}
