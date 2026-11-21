package handlers_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/handlers"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server/middleware"
	"github.com/ivanSaichkin/rag-movie-picker/backend/pkg/testutils"
)

type fakeRecommendationService struct {
	result  models.RecommendationResult
	err     error
	calls   int
	userID  uuid.UUID
	request models.RecommendationRequest
}

func (f *fakeRecommendationService) Recommend(_ context.Context, userID uuid.UUID, request models.RecommendationRequest) (models.RecommendationResult, error) {
	f.calls++
	f.userID = userID
	f.request = request
	return f.result, f.err
}

func bearerHeader() http.Header {
	header := http.Header{}
	header.Set("Authorization", "Bearer token")
	return header
}

func jsonWithAuth() http.Header {
	header := jsonHeader()
	header.Set("Authorization", "Bearer token")
	return header
}

func newRecommendationEngine(service *fakeRecommendationService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.Logging(slog.New(slog.NewJSONHandler(io.Discard, nil))))
	engine.POST("/recommendations", middleware.Authenticate(fakeIssuer{userID: testUserID}), handlers.NewRecommendationHandler(service).Recommend)
	return engine
}

func sampleRecommendationResult() models.RecommendationResult {
	historyID := uuid.New()
	year := int32(1999)
	rating := 8.2
	reason := "подходит под запрос"
	return models.RecommendationResult{
		HistoryID: &historyID,
		Response: models.Recommendation{
			Mode:         "generated",
			StrategyUsed: "hybrid",
			Items: []models.RecommendedMovie{{
				Movie: models.MovieCard{
					MovieID:   603,
					Title:     "The Matrix",
					Year:      &year,
					Genres:    []models.Genre{{ID: 28, Name: "Action"}},
					Rating:    &rating,
					VoteCount: 24589,
					Overview:  "A hacker discovers the truth.",
				},
				Reason: &reason,
			}},
			Warnings: []models.Warning{{Code: "FEWER_RESULTS", Message: "меньше"}},
		},
	}
}

func TestRecommendationReturnsResult(t *testing.T) {
	service := &fakeRecommendationService{result: sampleRecommendationResult()}
	engine := newRecommendationEngine(service)

	body := `{"query":"космос","limit":5,"filters":{"genre_ids":[878],"min_rating":6.5}}`
	recorder := testutils.Perform(t, engine, http.MethodPost, "/recommendations", strings.NewReader(body), jsonWithAuth())
	testutils.RequireStatus(t, recorder, http.StatusOK)

	response := testutils.DecodeJSON[dto.RecommendationResponse](t, recorder)
	if response.HistoryID == nil {
		t.Fatal("history_id is missing")
	}
	if response.Mode != "generated" || response.StrategyUsed != "hybrid" {
		t.Fatalf("response = %+v", response.StoredRecommendation)
	}
	if len(response.Items) != 1 || response.Items[0].MovieID != 603 {
		t.Fatalf("items = %+v", response.Items)
	}
	if response.Items[0].Reason == nil || *response.Items[0].Reason == "" {
		t.Fatalf("reason = %v", response.Items[0].Reason)
	}
	if len(response.Warnings) != 1 || response.Warnings[0].Code != "FEWER_RESULTS" {
		t.Fatalf("warnings = %+v", response.Warnings)
	}

	if service.calls != 1 || service.userID != testUserID {
		t.Fatalf("service calls = %d, user = %v", service.calls, service.userID)
	}
	if service.request.Query != "космос" || service.request.Limit == nil || *service.request.Limit != 5 {
		t.Fatalf("request = %+v", service.request)
	}
	if service.request.Filters.GenreIDs[0] != 878 || service.request.Filters.MinRating == nil {
		t.Fatalf("filters = %+v", service.request.Filters)
	}
}

func TestRecommendationWithoutHistoryIDSetsNull(t *testing.T) {
	result := sampleRecommendationResult()
	result.HistoryID = nil
	result.Response.Warnings = append(result.Response.Warnings, models.Warning{
		Code:    models.WarningHistoryNotSaved,
		Message: "История не сохранена.",
	})
	engine := newRecommendationEngine(&fakeRecommendationService{result: result})

	recorder := testutils.Perform(t, engine, http.MethodPost, "/recommendations",
		strings.NewReader(`{"query":"космос"}`), jsonWithAuth())
	testutils.RequireStatus(t, recorder, http.StatusOK)

	if !strings.Contains(recorder.Body.String(), `"history_id":null`) {
		t.Fatalf("history_id must be null: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), models.WarningHistoryNotSaved) {
		t.Fatalf("HISTORY_NOT_SAVED warning is missing: %s", recorder.Body.String())
	}
}

func TestRecommendationMapsValidationError(t *testing.T) {
	service := &fakeRecommendationService{err: &rules.ValidationError{Message: "Укажите запрос."}}
	engine := newRecommendationEngine(service)

	recorder := testutils.Perform(t, engine, http.MethodPost, "/recommendations",
		strings.NewReader(`{"query":""}`), jsonWithAuth())
	testutils.RequireStatus(t, recorder, http.StatusUnprocessableEntity)

	body := testutils.DecodeJSON[dto.ErrorResponse](t, recorder)
	if body.Error.Code != dto.CodeValidationError || body.Error.Message != "Укажите запрос." {
		t.Fatalf("error = %+v", body.Error)
	}
}

func TestRecommendationMapsDependencyFailures(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   dto.ErrorCode
	}{
		{"unavailable", rules.ErrUnavailable, http.StatusServiceUnavailable, dto.CodeDependencyUnavailable},
		{"deadline", rules.ErrDeadlineExceeded, http.StatusGatewayTimeout, dto.CodeDeadlineExceeded},
		{"unexpected", errors.New("boom"), http.StatusInternalServerError, dto.CodeInternal},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			engine := newRecommendationEngine(&fakeRecommendationService{err: testCase.err})

			recorder := testutils.Perform(t, engine, http.MethodPost, "/recommendations",
				strings.NewReader(`{"query":"космос"}`), jsonWithAuth())
			testutils.RequireStatus(t, recorder, testCase.status)

			body := testutils.DecodeJSON[dto.ErrorResponse](t, recorder)
			if body.Error.Code != testCase.code {
				t.Fatalf("code = %q, want %q", body.Error.Code, testCase.code)
			}
		})
	}
}

func TestRecommendationRejectsMalformedJSON(t *testing.T) {
	service := &fakeRecommendationService{}
	engine := newRecommendationEngine(service)

	recorder := testutils.Perform(t, engine, http.MethodPost, "/recommendations", strings.NewReader(`{`), jsonWithAuth())
	testutils.RequireStatus(t, recorder, http.StatusUnprocessableEntity)
	if service.calls != 0 {
		t.Fatal("malformed JSON must not reach the service")
	}
}

func TestRecommendationRequiresAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.Logging(slog.New(slog.NewJSONHandler(io.Discard, nil))))
	engine.POST("/recommendations", handlers.NewRecommendationHandler(&fakeRecommendationService{}).Recommend)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/recommendations", strings.NewReader(`{"query":"космос"}`)))
	testutils.RequireStatus(t, recorder, http.StatusUnauthorized)
}
