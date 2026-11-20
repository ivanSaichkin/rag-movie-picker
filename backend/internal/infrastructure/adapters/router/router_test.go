package router_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/adapters/router"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/handlers"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server/middleware"
	"github.com/ivanSaichkin/rag-movie-picker/backend/pkg/testutils"
)

const testOrigin = "http://127.0.0.1:8080"

// stubAuthService satisfies the contract for routing tests: handlers are not
// expected to produce successful responses here.
type stubAuthService struct{}

func (stubAuthService) Register(context.Context, string, string) (models.Session, error) {
	return models.Session{}, nil
}

func (stubAuthService) Login(context.Context, string, string) (models.Session, error) {
	return models.Session{}, nil
}

func (stubAuthService) Refresh(context.Context, string) (models.Session, error) {
	return models.Session{}, nil
}

func (stubAuthService) Logout(context.Context, string) error { return nil }

func (stubAuthService) CurrentUser(context.Context, uuid.UUID) (models.User, error) {
	return models.User{}, nil
}

type stubIssuer struct{}

func (stubIssuer) IssueAccessToken(uuid.UUID) (string, int, error) { return "token", 60, nil }

func (stubIssuer) ParseAccessToken(string) (uuid.UUID, error) {
	return uuid.Nil, rules.ErrUnauthenticated
}

func newTestRouter() http.Handler {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	return router.New(router.Params{
		Logger:        logger,
		Health:        handlers.NewHealthHandler(),
		Auth:          handlers.NewAuthHandler(stubAuthService{}, false),
		Authenticate:  middleware.Authenticate(stubIssuer{}),
		AllowedOrigin: testOrigin,
	})
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

func TestAuthRouteRejectsUnknownJSONFields(t *testing.T) {
	handler := newTestRouter()

	body := strings.NewReader(`{"email":"user@example.com","password":"password","admin":true}`)
	recorder := testutils.Perform(t, handler, http.MethodPost, "/api/v1/auth/register", body, nil)
	testutils.RequireStatus(t, recorder, http.StatusUnprocessableEntity)

	envelope := testutils.DecodeJSON[dto.ErrorResponse](t, recorder)
	if envelope.Error.Code != dto.CodeValidationError {
		t.Fatalf("code = %q, want %q", envelope.Error.Code, dto.CodeValidationError)
	}
}

func TestMeRequiresAuthorization(t *testing.T) {
	handler := newTestRouter()

	recorder := testutils.Perform(t, handler, http.MethodGet, "/api/v1/auth/me", nil, nil)
	testutils.RequireStatus(t, recorder, http.StatusUnauthorized)

	envelope := testutils.DecodeJSON[dto.ErrorResponse](t, recorder)
	if envelope.Error.Code != dto.CodeUnauthenticated {
		t.Fatalf("code = %q, want %q", envelope.Error.Code, dto.CodeUnauthenticated)
	}
}

func TestForeignOriginIsRejected(t *testing.T) {
	handler := newTestRouter()

	header := http.Header{}
	header.Set("Origin", "https://evil.example")

	recorder := testutils.Perform(t, handler, http.MethodPost, "/api/v1/auth/logout", nil, header)
	testutils.RequireStatus(t, recorder, http.StatusForbidden)

	envelope := testutils.DecodeJSON[dto.ErrorResponse](t, recorder)
	if envelope.Error.Code != dto.CodeForbidden {
		t.Fatalf("code = %q, want %q", envelope.Error.Code, dto.CodeForbidden)
	}
}
