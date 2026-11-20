package router_test

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/ivanSaichkin/rag-movie-picker/internal/infrastructure/adapters/router"
	"github.com/ivanSaichkin/rag-movie-picker/internal/interfaces/http/dto"
	"github.com/ivanSaichkin/rag-movie-picker/internal/interfaces/http/handlers"
	"github.com/ivanSaichkin/rag-movie-picker/internal/interfaces/http/server/middleware"
	"github.com/ivanSaichkin/rag-movie-picker/pkg/testutils"
)

func newTestRouter() http.Handler {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return router.New(logger, handlers.NewHealth())
}

func TestHealthRoute(t *testing.T) {
	handler := newTestRouter()

	recorder := testutils.Perform(t, handler, http.MethodGet, "/health", nil, nil)
	testutils.RequireStatus(t, recorder, http.StatusOK)

	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q, want JSON", contentType)
	}
	if recorder.Header().Get(middleware.HeaderRequestID) == "" {
		t.Fatal("response must carry X-Request-ID")
	}

	body := testutils.DecodeJSON[dto.HealthResponse](t, recorder)
	if body.Status != "ok" {
		t.Fatalf("status = %q, want ok", body.Status)
	}
}

func TestRequestIDIsEchoed(t *testing.T) {
	handler := newTestRouter()

	header := http.Header{}
	header.Set(middleware.HeaderRequestID, "req-42")

	recorder := testutils.Perform(t, handler, http.MethodGet, "/health", nil, header)
	if got := recorder.Header().Get(middleware.HeaderRequestID); got != "req-42" {
		t.Fatalf("X-Request-ID = %q, want req-42", got)
	}
}

func TestUnknownRouteReturnsJSONError(t *testing.T) {
	handler := newTestRouter()

	recorder := testutils.Perform(t, handler, http.MethodGet, "/api/v1/nope", nil, nil)
	testutils.RequireStatus(t, recorder, http.StatusNotFound)

	body := testutils.DecodeJSON[dto.ErrorResponse](t, recorder)
	if body.Error.Code != dto.CodeNotFound {
		t.Fatalf("code = %q, want %q", body.Error.Code, dto.CodeNotFound)
	}
}

func TestUnsupportedMethodReturnsJSONError(t *testing.T) {
	handler := newTestRouter()

	recorder := testutils.Perform(t, handler, http.MethodPost, "/health", nil, nil)
	testutils.RequireStatus(t, recorder, http.StatusMethodNotAllowed)

	body := testutils.DecodeJSON[dto.ErrorResponse](t, recorder)
	if body.Error.Code != dto.CodeMethodNotAllowed {
		t.Fatalf("code = %q, want %q", body.Error.Code, dto.CodeMethodNotAllowed)
	}
}

func TestTrailingSlashIsNotRedirected(t *testing.T) {
	handler := newTestRouter()

	recorder := testutils.Perform(t, handler, http.MethodGet, "/health/", nil, nil)
	testutils.RequireStatus(t, recorder, http.StatusNotFound)
}
