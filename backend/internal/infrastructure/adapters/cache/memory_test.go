package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/adapters/cache"
)

func TestMemorySetGetDelete(t *testing.T) {
	store := cache.NewInMemoryCache()
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

func TestMemoryExpiresValues(t *testing.T) {
	store := cache.NewInMemoryCache()
	ctx := context.Background()

	if err := store.Set(ctx, "key", []byte("value"), time.Millisecond); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	time.Sleep(5 * time.Millisecond)

	if _, found, err := store.Get(ctx, "key"); err != nil || found {
		t.Fatalf("Get() after expiry = %t, %v", found, err)
	}
}

func TestMemoryIncrCountsWithinWindow(t *testing.T) {
	store := cache.NewInMemoryCache()
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
}

func TestMemoryIncrResetsAfterWindow(t *testing.T) {
	store := cache.NewInMemoryCache()
	ctx := context.Background()

	first, err := store.Increment(ctx, "counter", 10*time.Millisecond)
	if err != nil || first != 1 {
		t.Fatalf("Increment() = %d, %v", first, err)
	}
	time.Sleep(20 * time.Millisecond)

	value, err := store.Increment(ctx, "counter", 10*time.Millisecond)
	if err != nil {
		t.Fatalf("Increment() error = %v", err)
	}
	if value != 1 {
		t.Fatalf("Increment() after window = %d, want 1", value)
	}
}
