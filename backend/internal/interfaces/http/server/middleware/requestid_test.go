package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server/middleware"
)

func TestRequestIDGeneratesIdentifier(t *testing.T) {
	engine := newRequestIDEngine()

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	header := recorder.Header().Get(middleware.HeaderRequestID)
	if header == "" {
		t.Fatal("response must carry X-Request-ID")
	}
	if _, err := uuid.Parse(header); err != nil {
		t.Fatalf("generated identifier %q is not a UUID: %v", header, err)
	}
	if body := recorder.Body.String(); body != header {
		t.Fatalf("body = %q, want %q", body, header)
	}
}

func TestRequestIDKeepsValidUpstreamIdentifier(t *testing.T) {
	engine := newRequestIDEngine()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	request.Header.Set(middleware.HeaderRequestID, "req-1.abc:def")
	engine.ServeHTTP(recorder, request)

	if got := recorder.Header().Get(middleware.HeaderRequestID); got != "req-1.abc:def" {
		t.Fatalf("request id = %q, want the upstream value", got)
	}
}

func TestRequestIDReplacesInvalidIdentifier(t *testing.T) {
	engine := newRequestIDEngine()

	for _, invalid := range []string{"has space", "semicolon;drop", string(make([]byte, 65))} {
		t.Run(invalid, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
			request.Header.Set(middleware.HeaderRequestID, invalid)
			engine.ServeHTTP(recorder, request)

			got := recorder.Header().Get(middleware.HeaderRequestID)
			if got == invalid {
				t.Fatalf("invalid identifier %q must be replaced", invalid)
			}
			if _, err := uuid.Parse(got); err != nil {
				t.Fatalf("replacement %q is not a UUID: %v", got, err)
			}
		})
	}
}

func newRequestIDEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.RequestID())
	engine.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, middleware.RequestIDFrom(c))
	})
	return engine
}
