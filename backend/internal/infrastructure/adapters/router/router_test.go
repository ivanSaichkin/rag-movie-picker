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
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/adapters/cache"
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

func (stubIssuer) ParseAccessToken(raw string) (uuid.UUID, error) {
	if raw == "valid-token" {
		return testUserID, nil
	}
	return uuid.Nil, rules.ErrUnauthenticated
}

type stubCatalogService struct {
	catalog    models.Catalog
	catalogErr error
	movie      models.MovieCard
	movieErr   error
}

func (s stubCatalogService) Catalog(context.Context) (models.Catalog, error) {
	return s.catalog, s.catalogErr
}

func (s stubCatalogService) Movie(context.Context, int64) (models.MovieCard, error) {
	return s.movie, s.movieErr
}

type stubRecommendationService struct{}

func (stubRecommendationService) RecommendGuest(context.Context, models.RecommendationRequest) (models.RecommendationResult, error) {
	return models.RecommendationResult{Response: models.Recommendation{Mode: "retrieval_only"}}, nil
}

func (stubRecommendationService) Recommend(context.Context, uuid.UUID, models.RecommendationRequest) (models.RecommendationResult, error) {
	return models.RecommendationResult{Response: models.Recommendation{
		Mode:     "empty",
		Items:    []models.RecommendedMovie{},
		Warnings: []models.Warning{},
	}}, nil
}

type stubUserMovieService struct{}

func (stubUserMovieService) Set(_ context.Context, _ uuid.UUID, movieID int64, update models.UserMovieUpdate) (models.UserMovie, error) {
	return models.UserMovie{MovieID: movieID, Status: update.Status, Movie: models.MovieCard{MovieID: movieID, Title: "Alien"}}, nil
}

func (stubUserMovieService) Delete(context.Context, uuid.UUID, int64) error { return nil }

func (stubUserMovieService) List(_ context.Context, _ uuid.UUID, _ models.UserMovieStatus, page, pageSize int) (models.UserMoviePage, error) {
	return models.UserMoviePage{Page: page, PageSize: pageSize, Items: []models.UserMovie{}}, nil
}

type stubHistoryService struct{}

func (stubHistoryService) List(_ context.Context, _ uuid.UUID, page, pageSize int) (models.HistoryPage, error) {
	return models.HistoryPage{Page: page, PageSize: pageSize, Items: []models.HistoryListItem{}}, nil
}

func (stubHistoryService) Get(context.Context, uuid.UUID, uuid.UUID) (models.HistoryEntry, error) {
	return models.HistoryEntry{}, rules.ErrNotFound
}

func (stubHistoryService) Delete(context.Context, uuid.UUID, uuid.UUID) error { return nil }

var testUserID = uuid.New()

func newTestRouter() http.Handler {
	return newTestRouterWithCatalog(stubCatalogService{
		catalog: models.Catalog{MovieCount: 4803, DefaultResults: 10, MaxResults: 50, DefaultStrategy: "hybrid"},
		movie:   models.MovieCard{MovieID: 1, Title: "Alien"},
	})
}

func newTestRouterWithCatalog(catalogService stubCatalogService) http.Handler {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	return router.New(router.Params{
		Logger:         logger,
		Health:         handlers.NewHealthHandler(),
		Auth:           handlers.NewAuthHandler(stubAuthService{}, false),
		Catalog:        handlers.NewCatalogHandler(catalogService),
		Recommendation: handlers.NewRecommendationHandler(stubRecommendationService{}),
		History:        handlers.NewHistoryHandler(stubHistoryService{}),
		UserMovies:     handlers.NewUserMovieHandler(stubUserMovieService{}),
		Authenticate:   middleware.Authenticate(stubIssuer{}),
		AllowedOrigin:  testOrigin,
	})
}

func authorizedHeader() http.Header {
	header := http.Header{}
	header.Set("Authorization", "Bearer valid-token")
	return header
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
	recorder := testutils.Perform(t, handler, http.MethodPost, "/api/v1/auth/login", body, nil)
	testutils.RequireStatus(t, recorder, http.StatusUnprocessableEntity)

	envelope := testutils.DecodeJSON[dto.ErrorResponse](t, recorder)
	if envelope.Error.Code != dto.CodeValidationError {
		t.Fatalf("code = %q, want %q", envelope.Error.Code, dto.CodeValidationError)
	}
}

func TestRegistrationIsDisabledByDefault(t *testing.T) {
	handler := newTestRouter()
	body := strings.NewReader(`{"email":"new@example.test","password":"password123"}`)
	recorder := testutils.Perform(t, handler, http.MethodPost, "/api/v1/auth/register", body, nil)
	testutils.RequireStatus(t, recorder, http.StatusNotFound)
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

func TestCatalogRequiresAuthorization(t *testing.T) {
	handler := newTestRouter()

	recorder := testutils.Perform(t, handler, http.MethodGet, "/api/v1/catalog", nil, nil)
	testutils.RequireStatus(t, recorder, http.StatusUnauthorized)
}

func TestCatalogReturnsData(t *testing.T) {
	handler := newTestRouter()

	recorder := testutils.Perform(t, handler, http.MethodGet, "/api/v1/catalog", nil, authorizedHeader())
	testutils.RequireStatus(t, recorder, http.StatusOK)

	body := testutils.DecodeJSON[dto.CatalogResponse](t, recorder)
	if body.MovieCount != 4803 || body.DefaultStrategy != "hybrid" {
		t.Fatalf("catalog = %+v", body)
	}
}

func TestMovieReturnsData(t *testing.T) {
	handler := newTestRouter()

	recorder := testutils.Perform(t, handler, http.MethodGet, "/api/v1/movies/1", nil, authorizedHeader())
	testutils.RequireStatus(t, recorder, http.StatusOK)

	body := testutils.DecodeJSON[dto.MovieResponse](t, recorder)
	if body.MovieID != 1 || body.Title != "Alien" {
		t.Fatalf("movie = %+v", body)
	}
}

func TestMovieRejectsInvalidID(t *testing.T) {
	handler := newTestRouter()

	recorder := testutils.Perform(t, handler, http.MethodGet, "/api/v1/movies/abc", nil, authorizedHeader())
	testutils.RequireStatus(t, recorder, http.StatusUnprocessableEntity)
}

func TestMovieMapsNotFound(t *testing.T) {
	catalogService := stubCatalogService{movieErr: rules.ErrNotFound}
	handler := newTestRouterWithCatalog(catalogService)

	recorder := testutils.Perform(t, handler, http.MethodGet, "/api/v1/movies/42", nil, authorizedHeader())
	testutils.RequireStatus(t, recorder, http.StatusNotFound)

	body := testutils.DecodeJSON[dto.ErrorResponse](t, recorder)
	if body.Error.Code != dto.CodeNotFound {
		t.Fatalf("code = %q", body.Error.Code)
	}
}

func TestRecommendationRouteRequiresAuthorization(t *testing.T) {
	handler := newTestRouter()

	recorder := testutils.Perform(t, handler, http.MethodPost, "/api/v1/recommendations", strings.NewReader(`{"query":"космос"}`), nil)
	testutils.RequireStatus(t, recorder, http.StatusUnauthorized)
}

func TestGuestRoutesWithoutAuthorization(t *testing.T) {
	handler := newTestRouter()
	catalog := testutils.Perform(t, handler, http.MethodGet, "/api/v1/guest/catalog", nil, nil)
	testutils.RequireStatus(t, catalog, http.StatusOK)
	if testutils.DecodeJSON[dto.CatalogResponse](t, catalog).MovieCount != 4803 {
		t.Fatal("guest catalog missing movie count")
	}
	request := strings.NewReader(`{"query":"space","disable_llm":false,"generate_explanations":true}`)
	recorder := testutils.Perform(t, handler, http.MethodPost, "/api/v1/guest/recommendations", request, nil)
	testutils.RequireStatus(t, recorder, http.StatusOK)
	body := testutils.DecodeJSON[dto.RecommendationResponse](t, recorder)
	if body.Mode != "retrieval_only" || body.HistoryID != nil {
		t.Fatalf("guest response = %+v", body)
	}
}

func TestGuestSearchRateLimit(t *testing.T) {
	handler := router.New(router.Params{
		Logger:         slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Auth:           handlers.NewAuthHandler(stubAuthService{}, false),
		Health:         handlers.NewHealthHandler(),
		Recommendation: handlers.NewRecommendationHandler(stubRecommendationService{}),
		Limiters: router.Limiters{
			Guest: middleware.GuestSearchRateLimit(cache.NewInMemoryCache(), 1),
		},
	})
	for _, status := range []int{http.StatusOK, http.StatusTooManyRequests} {
		header := http.Header{}
		if status == http.StatusTooManyRequests {
			header.Set("X-Forwarded-For", "192.0.2.72")
		}
		response := testutils.Perform(t, handler, http.MethodPost, "/api/v1/guest/recommendations", strings.NewReader(`{"query":"space"}`), header)
		testutils.RequireStatus(t, response, status)
		if status == http.StatusTooManyRequests && response.Header().Get("Retry-After") == "" {
			t.Fatal("missing Retry-After for guest search")
		}
	}
}

func TestRecommendationRouteReturnsResult(t *testing.T) {
	handler := newTestRouter()

	header := authorizedHeader()
	header.Set("Content-Type", "application/json")
	recorder := testutils.Perform(t, handler, http.MethodPost, "/api/v1/recommendations", strings.NewReader(`{"query":"космос"}`), header)
	testutils.RequireStatus(t, recorder, http.StatusOK)

	body := testutils.DecodeJSON[dto.RecommendationResponse](t, recorder)
	if body.Mode != "empty" || body.HistoryID != nil {
		t.Fatalf("response = %+v", body.StoredRecommendation)
	}
}

func TestHistoryRouteRequiresAuthorization(t *testing.T) {
	handler := newTestRouter()

	recorder := testutils.Perform(t, handler, http.MethodGet, "/api/v1/history", nil, nil)
	testutils.RequireStatus(t, recorder, http.StatusUnauthorized)
}

func TestHistoryRouteReturnsPage(t *testing.T) {
	handler := newTestRouter()

	recorder := testutils.Perform(t, handler, http.MethodGet, "/api/v1/history?page=2&page_size=10", nil, authorizedHeader())
	testutils.RequireStatus(t, recorder, http.StatusOK)

	body := testutils.DecodeJSON[dto.HistoryListResponse](t, recorder)
	if body.Page != 2 || body.PageSize != 10 || body.Total != 0 {
		t.Fatalf("page = %+v", body)
	}
}

func TestHistoryDeleteRoute(t *testing.T) {
	handler := newTestRouter()

	recorder := testutils.Perform(t, handler, http.MethodDelete, "/api/v1/history/"+uuid.NewString(), nil, authorizedHeader())
	testutils.RequireStatus(t, recorder, http.StatusNoContent)
}

func TestHistoryGetRouteMapsNotFound(t *testing.T) {
	handler := newTestRouter()

	recorder := testutils.Perform(t, handler, http.MethodGet, "/api/v1/history/"+uuid.NewString(), nil, authorizedHeader())
	testutils.RequireStatus(t, recorder, http.StatusNotFound)
}

func TestUserMoviesRouteRequiresAuthorization(t *testing.T) {
	handler := newTestRouter()

	recorder := testutils.Perform(t, handler, http.MethodGet, "/api/v1/my/movies?status=watched", nil, nil)
	testutils.RequireStatus(t, recorder, http.StatusUnauthorized)
}

func TestUserMoviesRouteReturnsPage(t *testing.T) {
	handler := newTestRouter()

	recorder := testutils.Perform(t, handler, http.MethodGet, "/api/v1/my/movies?status=watched", nil, authorizedHeader())
	testutils.RequireStatus(t, recorder, http.StatusOK)

	body := testutils.DecodeJSON[dto.UserMovieListResponse](t, recorder)
	if body.Page != 1 || body.PageSize != 20 {
		t.Fatalf("page = %+v", body)
	}
}

func TestUserMovieSetRoute(t *testing.T) {
	handler := newTestRouter()

	header := authorizedHeader()
	header.Set("Content-Type", "application/json")
	recorder := testutils.Perform(t, handler, http.MethodPut, "/api/v1/my/movies/603",
		strings.NewReader(`{"status":"watched","liked":true}`), header)
	testutils.RequireStatus(t, recorder, http.StatusOK)

	body := testutils.DecodeJSON[dto.UserMovieResponse](t, recorder)
	if body.MovieID != 603 || body.Status != "watched" {
		t.Fatalf("response = %+v", body)
	}
}

func TestUserMovieDeleteRoute(t *testing.T) {
	handler := newTestRouter()

	recorder := testutils.Perform(t, handler, http.MethodDelete, "/api/v1/my/movies/603", nil, authorizedHeader())
	testutils.RequireStatus(t, recorder, http.StatusNoContent)
}
