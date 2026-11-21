package handlers_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/handlers"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server/middleware"
	"github.com/ivanSaichkin/rag-movie-picker/backend/pkg/testutils"
)

type fakeUserMovieService struct {
	movie     models.UserMovie
	setErr    error
	page      models.UserMoviePage
	listErr   error
	deleteErr error

	setCalls     int
	setMovieID   int64
	setUpdate    models.UserMovieUpdate
	listCalls    int
	listStatus   models.UserMovieStatus
	listPage     int
	listPageSize int
	deleteCalls  int
	deleteID     int64
}

func (f *fakeUserMovieService) Set(_ context.Context, _ uuid.UUID, movieID int64, update models.UserMovieUpdate) (models.UserMovie, error) {
	f.setCalls++
	f.setMovieID = movieID
	f.setUpdate = update
	return f.movie, f.setErr
}

func (f *fakeUserMovieService) Delete(_ context.Context, _ uuid.UUID, movieID int64) error {
	f.deleteCalls++
	f.deleteID = movieID
	return f.deleteErr
}

func (f *fakeUserMovieService) List(_ context.Context, _ uuid.UUID, status models.UserMovieStatus, page, pageSize int) (models.UserMoviePage, error) {
	f.listCalls++
	f.listStatus = status
	f.listPage = page
	f.listPageSize = pageSize
	return f.page, f.listErr
}

func newUserMovieEngine(service *fakeUserMovieService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.Logging(slog.New(slog.NewJSONHandler(io.Discard, nil))))

	handler := handlers.NewUserMovieHandler(service)
	engine.GET("/my/movies", middleware.Authenticate(fakeIssuer{userID: testUserID}), handler.List)
	engine.PUT("/my/movies/:movie_id", middleware.Authenticate(fakeIssuer{userID: testUserID}), handler.Set)
	engine.DELETE("/my/movies/:movie_id", middleware.Authenticate(fakeIssuer{userID: testUserID}), handler.Delete)
	return engine
}

func sampleListedMovie() models.UserMovie {
	year := int32(1999)
	rating := 8.2
	liked := true
	return models.UserMovie{
		UserID:  testUserID,
		MovieID: 603,
		Status:  models.UserMovieWatched,
		Liked:   &liked,
		Movie: models.MovieCard{
			MovieID:   603,
			Title:     "The Matrix",
			Year:      &year,
			Genres:    []models.Genre{{ID: 28, Name: "Action"}},
			Rating:    &rating,
			VoteCount: 24589,
			Overview:  "A hacker discovers the truth.",
		},
		UpdatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

func TestUserMovieSetReturnsEntry(t *testing.T) {
	service := &fakeUserMovieService{movie: sampleListedMovie()}
	engine := newUserMovieEngine(service)

	recorder := testutils.Perform(t, engine, http.MethodPut, "/my/movies/603",
		strings.NewReader(`{"status":"watched","liked":true}`), jsonWithAuth())
	testutils.RequireStatus(t, recorder, http.StatusOK)

	body := testutils.DecodeJSON[dto.UserMovieResponse](t, recorder)
	if body.MovieID != 603 || body.Title != "The Matrix" || body.Status != "watched" {
		t.Fatalf("response = %+v", body)
	}
	if body.Liked == nil || !*body.Liked {
		t.Fatalf("liked = %v", body.Liked)
	}
	if body.UpdatedAt.IsZero() {
		t.Fatal("updated_at is missing")
	}

	if service.setCalls != 1 || service.setMovieID != 603 {
		t.Fatalf("service call = %d, movie id = %d", service.setCalls, service.setMovieID)
	}
	if service.setUpdate.Status != models.UserMovieWatched || service.setUpdate.Liked == nil {
		t.Fatalf("update = %+v", service.setUpdate)
	}
}

func TestUserMovieSetWithoutRating(t *testing.T) {
	movie := sampleListedMovie()
	movie.Status = models.UserMovieWantToWatch
	movie.Liked = nil
	engine := newUserMovieEngine(&fakeUserMovieService{movie: movie})

	recorder := testutils.Perform(t, engine, http.MethodPut, "/my/movies/603",
		strings.NewReader(`{"status":"want_to_watch"}`), jsonWithAuth())
	testutils.RequireStatus(t, recorder, http.StatusOK)

	if !strings.Contains(recorder.Body.String(), `"liked":null`) {
		t.Fatalf("liked must be null: %s", recorder.Body.String())
	}
}

func TestUserMovieSetRejectsInvalidID(t *testing.T) {
	for _, movieID := range []string{"abc", "0", "-3"} {
		t.Run(movieID, func(t *testing.T) {
			service := &fakeUserMovieService{}
			engine := newUserMovieEngine(service)

			recorder := testutils.Perform(t, engine, http.MethodPut, "/my/movies/"+movieID,
				strings.NewReader(`{"status":"watched"}`), jsonWithAuth())
			testutils.RequireStatus(t, recorder, http.StatusUnprocessableEntity)
			if service.setCalls != 0 {
				t.Fatal("invalid id must not reach the service")
			}
		})
	}
}

func TestUserMovieSetMapsValidationError(t *testing.T) {
	service := &fakeUserMovieService{setErr: &rules.ValidationError{Message: "Оценка доступна только для просмотренного фильма."}}
	engine := newUserMovieEngine(service)

	recorder := testutils.Perform(t, engine, http.MethodPut, "/my/movies/603",
		strings.NewReader(`{"status":"want_to_watch","liked":true}`), jsonWithAuth())
	testutils.RequireStatus(t, recorder, http.StatusUnprocessableEntity)

	body := testutils.DecodeJSON[dto.ErrorResponse](t, recorder)
	if body.Error.Code != dto.CodeValidationError || body.Error.Message != "Оценка доступна только для просмотренного фильма." {
		t.Fatalf("error = %+v", body.Error)
	}
}

func TestUserMovieSetMapsDependencyFailures(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   dto.ErrorCode
	}{
		{"movie not found", rules.ErrNotFound, http.StatusNotFound, dto.CodeNotFound},
		{"unavailable", rules.ErrUnavailable, http.StatusServiceUnavailable, dto.CodeDependencyUnavailable},
		{"deadline", rules.ErrDeadlineExceeded, http.StatusGatewayTimeout, dto.CodeDeadlineExceeded},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			engine := newUserMovieEngine(&fakeUserMovieService{setErr: testCase.err})

			recorder := testutils.Perform(t, engine, http.MethodPut, "/my/movies/603",
				strings.NewReader(`{"status":"watched"}`), jsonWithAuth())
			testutils.RequireStatus(t, recorder, testCase.status)

			body := testutils.DecodeJSON[dto.ErrorResponse](t, recorder)
			if body.Error.Code != testCase.code {
				t.Fatalf("code = %q, want %q", body.Error.Code, testCase.code)
			}
		})
	}
}

func TestUserMovieListReturnsPage(t *testing.T) {
	service := &fakeUserMovieService{page: models.UserMoviePage{
		Items:    []models.UserMovie{sampleListedMovie()},
		Page:     1,
		PageSize: 20,
		Total:    1,
	}}
	engine := newUserMovieEngine(service)

	recorder := testutils.Perform(t, engine, http.MethodGet, "/my/movies?status=watched", nil, bearerHeader())
	testutils.RequireStatus(t, recorder, http.StatusOK)

	if service.listCalls != 1 || service.listStatus != models.UserMovieWatched {
		t.Fatalf("service status = %q, calls = %d", service.listStatus, service.listCalls)
	}
	if service.listPage != 1 || service.listPageSize != 20 {
		t.Fatalf("service page = %d, size = %d", service.listPage, service.listPageSize)
	}

	body := testutils.DecodeJSON[dto.UserMovieListResponse](t, recorder)
	if body.Total != 1 || len(body.Items) != 1 || body.Items[0].MovieID != 603 {
		t.Fatalf("response = %+v", body)
	}
}

func TestUserMovieListRequiresStatus(t *testing.T) {
	service := &fakeUserMovieService{}
	engine := newUserMovieEngine(service)

	recorder := testutils.Perform(t, engine, http.MethodGet, "/my/movies", nil, bearerHeader())
	testutils.RequireStatus(t, recorder, http.StatusUnprocessableEntity)

	body := testutils.DecodeJSON[dto.ErrorResponse](t, recorder)
	if body.Error.Code != dto.CodeValidationError {
		t.Fatalf("code = %q", body.Error.Code)
	}
	if service.listCalls != 0 {
		t.Fatal("request without status must not reach the service")
	}
}

func TestUserMovieListRejectsInvalidStatusAndPagination(t *testing.T) {
	for _, target := range []string{
		"/my/movies?status=maybe",
		"/my/movies?status=watched&page=0",
		"/my/movies?status=watched&page_size=101",
		"/my/movies?status=watched&page=abc",
	} {
		t.Run(target, func(t *testing.T) {
			service := &fakeUserMovieService{}
			engine := newUserMovieEngine(service)

			recorder := testutils.Perform(t, engine, http.MethodGet, target, nil, bearerHeader())
			testutils.RequireStatus(t, recorder, http.StatusUnprocessableEntity)
			if service.listCalls != 0 {
				t.Fatal("invalid query must not reach the service")
			}
		})
	}
}

func TestUserMovieDeleteReturnsNoContent(t *testing.T) {
	service := &fakeUserMovieService{}
	engine := newUserMovieEngine(service)

	recorder := testutils.Perform(t, engine, http.MethodDelete, "/my/movies/603", nil, bearerHeader())
	testutils.RequireStatus(t, recorder, http.StatusNoContent)

	if service.deleteCalls != 1 || service.deleteID != 603 {
		t.Fatalf("service delete calls = %d, id = %d", service.deleteCalls, service.deleteID)
	}
}

func TestUserMovieDeleteMapsNotFound(t *testing.T) {
	engine := newUserMovieEngine(&fakeUserMovieService{deleteErr: rules.ErrNotFound})

	recorder := testutils.Perform(t, engine, http.MethodDelete, "/my/movies/603", nil, bearerHeader())
	testutils.RequireStatus(t, recorder, http.StatusNotFound)

	body := testutils.DecodeJSON[dto.ErrorResponse](t, recorder)
	if body.Error.Code != dto.CodeNotFound {
		t.Fatalf("code = %q", body.Error.Code)
	}
}

func TestUserMovieRequiresAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.Logging(slog.New(slog.NewJSONHandler(io.Discard, nil))))
	engine.GET("/my/movies", handlers.NewUserMovieHandler(&fakeUserMovieService{}).List)

	recorder := testutils.Perform(t, engine, http.MethodGet, "/my/movies?status=watched", nil, nil)
	testutils.RequireStatus(t, recorder, http.StatusUnauthorized)
}
