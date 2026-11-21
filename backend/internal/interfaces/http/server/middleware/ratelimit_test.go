package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/adapters/cache"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server/middleware"
)

func newRateLimitEngine(limit int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.RateLimit(cache.NewInMemoryCache(), "login", limit, time.Minute))
	engine.GET("/resource", func(c *gin.Context) { c.Status(http.StatusOK) })
	return engine
}

func TestRateLimitAllowsRequestsUnderLimit(t *testing.T) {
	engine := newRateLimitEngine(2)

	for attempt := 1; attempt <= 2; attempt++ {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/resource", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("attempt %d: status = %d, want 200", attempt, recorder.Code)
		}
	}
}

func TestRateLimitRejectsRequestsOverLimit(t *testing.T) {
	engine := newRateLimitEngine(1)

	first := httptest.NewRecorder()
	engine.ServeHTTP(first, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/resource", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d", first.Code)
	}

	second := httptest.NewRecorder()
	engine.ServeHTTP(second, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/resource", nil))

	assertErrorCode(t, second, http.StatusTooManyRequests, dto.CodeRateLimited)
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After header is missing")
	}
}

func TestRateLimitSeparatesClientsByIP(t *testing.T) {
	engine := newRateLimitEngine(1)

	first := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/resource", nil)
	first.RemoteAddr = "203.0.113.7:4000"
	firstRecorder := httptest.NewRecorder()
	engine.ServeHTTP(firstRecorder, first)

	second := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/resource", nil)
	second.RemoteAddr = "198.51.100.9:4000"
	secondRecorder := httptest.NewRecorder()
	engine.ServeHTTP(secondRecorder, second)

	if secondRecorder.Code != http.StatusOK {
		t.Fatalf("another client status = %d, want 200", secondRecorder.Code)
	}
}

func TestRateLimitFailsOpenWhenCacheFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.RateLimit(failingCache{}, "login", 1, time.Minute))
	engine.GET("/resource", func(c *gin.Context) { c.Status(http.StatusOK) })

	for attempt := 1; attempt <= 2; attempt++ {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/resource", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("attempt %d: status = %d, want 200 when the cache fails", attempt, recorder.Code)
		}
	}
}

// failingCache simulates an unreachable cache.
type failingCache struct{}

func (failingCache) Get(context.Context, string) ([]byte, bool, error) {
	return nil, false, errors.New("cache is down")
}

func (failingCache) Set(context.Context, string, []byte, time.Duration) error {
	return errors.New("cache is down")
}

func (failingCache) Increment(context.Context, string, time.Duration) (int64, error) {
	return 0, errors.New("cache is down")
}

func (failingCache) Delete(context.Context, string) error {
	return errors.New("cache is down")
}
