package handlers_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ivanSaichkin/rag-movie-picker/internal/interfaces/http/dto"
	"github.com/ivanSaichkin/rag-movie-picker/internal/interfaces/http/handlers"
	"github.com/ivanSaichkin/rag-movie-picker/internal/interfaces/http/server/middleware"
	"github.com/ivanSaichkin/rag-movie-picker/pkg/testutils"
)

type fakeCheck struct {
	name string
	err  error
}

func (f fakeCheck) Name() string { return f.name }

func (f fakeCheck) Check(context.Context) error { return f.err }

func newHealthEngine(health *handlers.Health) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.Logging(slog.New(slog.NewJSONHandler(io.Discard, nil))))
	engine.GET("/health", health.Liveness)
	engine.GET("/ready", health.Readiness)
	return engine
}

func TestLiveness(t *testing.T) {
	engine := newHealthEngine(handlers.NewHealth())

	recorder := testutils.Perform(t, engine, http.MethodGet, "/health", nil, nil)
	testutils.RequireStatus(t, recorder, http.StatusOK)

	body := testutils.DecodeJSON[dto.HealthResponse](t, recorder)
	if body.Status != "ok" {
		t.Fatalf("status = %q, want ok", body.Status)
	}
}

func TestReadinessWithoutChecks(t *testing.T) {
	engine := newHealthEngine(handlers.NewHealth())

	recorder := testutils.Perform(t, engine, http.MethodGet, "/ready", nil, nil)
	testutils.RequireStatus(t, recorder, http.StatusOK)

	if strings.Contains(recorder.Body.String(), "failed_checks") {
		t.Fatalf("healthy response must omit failed_checks: %s", recorder.Body.String())
	}
	body := testutils.DecodeJSON[dto.ReadinessResponse](t, recorder)
	if body.Status != "ok" {
		t.Fatalf("status = %q, want ok", body.Status)
	}
}

func TestReadinessWithHealthyChecks(t *testing.T) {
	engine := newHealthEngine(handlers.NewHealth(fakeCheck{name: "postgres"}))

	recorder := testutils.Perform(t, engine, http.MethodGet, "/ready", nil, nil)
	testutils.RequireStatus(t, recorder, http.StatusOK)
}

func TestReadinessReportsFailedCheckNamesOnly(t *testing.T) {
	engine := newHealthEngine(handlers.NewHealth(
		fakeCheck{name: "postgres"},
		fakeCheck{name: "redis", err: errors.New("dial tcp 127.0.0.1:6379: connection refused")},
	))

	recorder := testutils.Perform(t, engine, http.MethodGet, "/ready", nil, nil)
	testutils.RequireStatus(t, recorder, http.StatusServiceUnavailable)

	raw := recorder.Body.String()
	if strings.Contains(raw, "connection refused") {
		t.Fatalf("dependency error must stay in the log, got %s", raw)
	}

	body := testutils.DecodeJSON[dto.ReadinessResponse](t, recorder)
	if body.Status != "unavailable" {
		t.Fatalf("status = %q, want unavailable", body.Status)
	}
	if len(body.FailedChecks) != 1 || body.FailedChecks[0] != "redis" {
		t.Fatalf("failed_checks = %v, want [redis]", body.FailedChecks)
	}
}

func TestReadinessKeepsCheckOrder(t *testing.T) {
	engine := newHealthEngine(handlers.NewHealth(
		fakeCheck{name: "postgres", err: errors.New("down")},
		fakeCheck{name: "redis", err: errors.New("down")},
	))

	recorder := testutils.Perform(t, engine, http.MethodGet, "/ready", nil, nil)

	body := testutils.DecodeJSON[dto.ReadinessResponse](t, recorder)
	if len(body.FailedChecks) != 2 || body.FailedChecks[0] != "postgres" || body.FailedChecks[1] != "redis" {
		t.Fatalf("failed_checks = %v, want [postgres redis]", body.FailedChecks)
	}
}
