// Package cache contains the key-value implementations used by rate limits and
// lookups. Redis is optional: the in-memory implementation is always available.
package cache

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"
)

const sweepEvery = 1024

type entry struct {
	value     []byte
	expiresAt time.Time
}

// Memory is an in-process cache. It is used when Redis is not configured and
// as the fallback when Redis fails.
type Memory struct {
	mu    sync.Mutex
	items map[string]entry
	ops   int
}

// NewMemory builds an empty in-memory cache.
func NewMemory() *Memory {
	return &Memory{items: make(map[string]entry)}
}

// Get returns a copy of the value and reports whether the key is present.
func (m *Memory) Get(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, ok := m.items[key]
	if !ok {
		return nil, false, nil
	}
	if time.Now().After(item.expiresAt) {
		delete(m.items, key)
		return nil, false, nil
	}
	return append([]byte(nil), item.value...), true, nil
}

// Set stores the value until the TTL elapses.
func (m *Memory) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.items[key] = entry{value: append([]byte(nil), value...), expiresAt: time.Now().Add(ttl)}
	m.maybeSweep()
	return nil
}

// Incr increments the counter and creates it with the TTL on the first call.
func (m *Memory) Incr(_ context.Context, key string, ttl time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	item, ok := m.items[key]
	if !ok || now.After(item.expiresAt) {
		m.items[key] = entry{value: []byte("1"), expiresAt: now.Add(ttl)}
		m.maybeSweep()
		return 1, nil
	}

	value, err := strconv.ParseInt(string(item.value), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("decode counter %q: %w", key, err)
	}
	value++
	m.items[key] = entry{value: []byte(strconv.FormatInt(value, 10)), expiresAt: item.expiresAt}
	m.maybeSweep()
	return value, nil
}

// Delete removes the key.
func (m *Memory) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.items, key)
	return nil
}

// maybeSweep drops expired entries from time to time so that a stream of
// unique keys does not grow the map without bound. The caller holds the lock.
func (m *Memory) maybeSweep() {
	m.ops++
	if m.ops < sweepEvery {
		return
	}
	m.ops = 0

	now := time.Now()
	for key, item := range m.items {
		if now.After(item.expiresAt) {
			delete(m.items, key)
		}
	}
}
