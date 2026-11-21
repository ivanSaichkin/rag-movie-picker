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

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/handlers"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server/middleware"
	"github.com/ivanSaichkin/rag-movie-picker/backend/pkg/testutils"
)

type fakeCatalogService struct {
	catalog    models.Catalog
	catalogErr error
	movie      models.MovieCard
	movieErr   error
	movieID    int64
}

func (f *fakeCatalogService) Catalog(context.Context) (models.Catalog, error) {
	return f.catalog, f.catalogErr
}

func (f *fakeCatalogService) Movie(_ context.Context, movieID int64) (models.MovieCard, error) {
	f.movieID = movieID
	return f.movie, f.movieErr
}

func newCatalogEngine(service *fakeCatalogService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.Logging(slog.New(slog.NewJSONHandler(io.Discard, nil))))

	handler := handlers.NewCatalogHandler(service)
	engine.GET("/catalog", handler.Catalog)
	engine.GET("/movies/:movie_id", handler.Movie)
	return engine
}

func catalogFixture() models.Catalog {
	yearFrom, yearTo := int32(1902), int32(2017)
	return models.Catalog{
		Genres:              []models.Genre{{ID: 878, Name: "Science Fiction"}},
		MovieCount:          4803,
		YearFrom:            &yearFrom,
		YearTo:              &yearTo,
		DefaultResults:      10,
		MaxResults:          50,
		AvailableStrategies: []string{"bm25", "transformer", "hybrid"},
		DefaultStrategy:     "hybrid",
	}
}

func TestCatalogReturnsMetadata(t *testing.T) {
	engine := newCatalogEngine(&fakeCatalogService{catalog: catalogFixture()})

	recorder := testutils.Perform(t, engine, http.MethodGet, "/catalog", nil, nil)
	testutils.RequireStatus(t, recorder, http.StatusOK)

	body := testutils.DecodeJSON[dto.CatalogResponse](t, recorder)
	if body.MovieCount != 4803 || body.DefaultResults != 10 || body.MaxResults != 50 {
		t.Fatalf("catalog = %+v", body)
	}
	if len(body.Genres) != 1 || body.Genres[0].ID != 878 {
		t.Fatalf("genres = %+v", body.Genres)
	}
	if body.YearFrom == nil || *body.YearFrom != 1902 {
		t.Fatalf("year_from = %v", body.YearFrom)
	}
	if body.DefaultStrategy != "hybrid" || len(body.AvailableStrategies) != 3 {
		t.Fatalf("strategies = %+v", body)
	}
}

func TestCatalogOmitsUnknownBounds(t *testing.T) {
	engine := newCatalogEngine(&fakeCatalogService{catalog: models.Catalog{MovieCount: 10}})

	recorder := testutils.Perform(t, engine, http.MethodGet, "/catalog", nil, nil)
	testutils.RequireStatus(t, recorder, http.StatusOK)

	if strings.Contains(recorder.Body.String(), "year_from") || strings.Contains(recorder.Body.String(), "year_to") {
		t.Fatalf("unknown bounds must be omitted: %s", recorder.Body.String())
	}
}

func TestCatalogMapsDependencyFailures(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   dto.ErrorCode
	}{
		{"unavailable", rules.ErrUnavailable, http.StatusServiceUnavailable, dto.CodeDependencyUnavailable},
		{"deadline", rules.ErrDeadlineExceeded, http.StatusGatewayTimeout, dto.CodeDeadlineExceeded},
		{"invalid argument", rules.ErrInvalidArgument, http.StatusUnprocessableEntity, dto.CodeValidationError},
		{"unexpected", errors.New("boom"), http.StatusInternalServerError, dto.CodeInternal},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			engine := newCatalogEngine(&fakeCatalogService{catalogErr: testCase.err})

			recorder := testutils.Perform(t, engine, http.MethodGet, "/catalog", nil, nil)
			testutils.RequireStatus(t, recorder, testCase.status)

			body := testutils.DecodeJSON[dto.ErrorResponse](t, recorder)
			if body.Error.Code != testCase.code {
				t.Fatalf("code = %q, want %q", body.Error.Code, testCase.code)
			}
		})
	}
}

func TestMovieReturnsCard(t *testing.T) {
	year := int32(1999)
	rating := 8.2
	engine := newCatalogEngine(&fakeCatalogService{movie: models.MovieCard{
		MovieID:   603,
		Title:     "The Matrix",
		Year:      &year,
		Genres:    []models.Genre{{ID: 28, Name: "Action"}},
		Rating:    &rating,
		VoteCount: 24589,
		Overview:  "A hacker discovers the truth.",
	}})

	recorder := testutils.Perform(t, engine, http.MethodGet, "/movies/603", nil, nil)
	testutils.RequireStatus(t, recorder, http.StatusOK)

	body := testutils.DecodeJSON[dto.MovieResponse](t, recorder)
	if body.MovieID != 603 || body.Title != "The Matrix" {
		t.Fatalf("movie = %+v", body)
	}
	if body.Year == nil || *body.Year != 1999 || body.Rating == nil || *body.Rating != 8.2 {
		t.Fatalf("optional fields = %v, %v", body.Year, body.Rating)
	}
}

func TestMovieOmitsUnknownOptionalFields(t *testing.T) {
	engine := newCatalogEngine(&fakeCatalogService{movie: models.MovieCard{MovieID: 1, Title: "Untitled"}})

	recorder := testutils.Perform(t, engine, http.MethodGet, "/movies/1", nil, nil)
	testutils.RequireStatus(t, recorder, http.StatusOK)

	for _, field := range []string{"\"year\"", "\"rating\""} {
		if strings.Contains(recorder.Body.String(), field) {
			t.Fatalf("unknown %s must be omitted: %s", field, recorder.Body.String())
		}
	}
}

func TestMovieRejectsInvalidID(t *testing.T) {
	for _, movieID := range []string{"abc", "0", "-3"} {
		t.Run(movieID, func(t *testing.T) {
			engine := newCatalogEngine(&fakeCatalogService{})

			recorder := testutils.Perform(t, engine, http.MethodGet, "/movies/"+movieID, nil, nil)
			testutils.RequireStatus(t, recorder, http.StatusUnprocessableEntity)

			body := testutils.DecodeJSON[dto.ErrorResponse](t, recorder)
			if body.Error.Code != dto.CodeValidationError {
				t.Fatalf("code = %q", body.Error.Code)
			}
		})
	}
}

func TestMovieMapsNotFound(t *testing.T) {
	engine := newCatalogEngine(&fakeCatalogService{movieErr: rules.ErrNotFound})

	recorder := testutils.Perform(t, engine, http.MethodGet, "/movies/999999", nil, nil)
	testutils.RequireStatus(t, recorder, http.StatusNotFound)

	body := testutils.DecodeJSON[dto.ErrorResponse](t, recorder)
	if body.Error.Code != dto.CodeNotFound {
		t.Fatalf("code = %q", body.Error.Code)
	}
}

func TestMovieMapsUnavailable(t *testing.T) {
	engine := newCatalogEngine(&fakeCatalogService{movieErr: rules.ErrUnavailable})

	recorder := testutils.Perform(t, engine, http.MethodGet, "/movies/603", nil, nil)
	testutils.RequireStatus(t, recorder, http.StatusServiceUnavailable)

	body := testutils.DecodeJSON[dto.ErrorResponse](t, recorder)
	if body.Error.Code != dto.CodeDependencyUnavailable {
		t.Fatalf("code = %q", body.Error.Code)
	}
}
