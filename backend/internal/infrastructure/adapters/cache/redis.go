package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// incrScript increments the counter and sets the expiry only on creation, so
// that the window does not slide on every request.
var incrScript = redis.NewScript(`
local value = redis.call('INCR', KEYS[1])
if value == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return value
`)

// RedisCache stores cache entries in Redis.
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache builds the client. The connection is lazy: a broken Redis does not
// block startup, the fallback cache handles the errors.
func NewRedisCache(addr, password string, db int, timeout time.Duration) *RedisCache {
	return &RedisCache{client: redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		DialTimeout:  timeout,
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
	})}
}

// Get returns the raw value and reports whether the key is present.
func (r *RedisCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	value, err := r.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("redis get %q: %w", key, err)
	}
	return value, true, nil
}

// Set stores the value with the TTL.
func (r *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := r.client.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("redis set %q: %w", key, err)
	}
	return nil
}

// Increment increments the counter, setting the expiry on creation.
func (r *RedisCache) Increment(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	value, err := incrScript.Run(ctx, r.client, []string{key}, ttl.Milliseconds()).Int64()
	if err != nil {
		return 0, fmt.Errorf("redis incr %q: %w", key, err)
	}
	return value, nil
}

// Delete removes the key.
func (r *RedisCache) Delete(ctx context.Context, key string) error {
	if err := r.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("redis delete %q: %w", key, err)
	}
	return nil
}

// Close releases the connection pool.
func (r *RedisCache) Close() error {
	if err := r.client.Close(); err != nil {
		return fmt.Errorf("close redis client: %w", err)
	}
	return nil
}
