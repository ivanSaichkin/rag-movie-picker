package handlers_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
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

type fakeHistoryService struct {
	page      models.HistoryPage
	listErr   error
	entry     models.HistoryEntry
	getErr    error
	deleteErr error

	calls        int
	userID       uuid.UUID
	pageArg      int
	pageSizeArg  int
	historyIDArg uuid.UUID
}

func (f *fakeHistoryService) List(_ context.Context, userID uuid.UUID, page, pageSize int) (models.HistoryPage, error) {
	f.calls++
	f.userID = userID
	f.pageArg = page
	f.pageSizeArg = pageSize
	return f.page, f.listErr
}

func (f *fakeHistoryService) Get(_ context.Context, userID, historyID uuid.UUID) (models.HistoryEntry, error) {
	f.calls++
	f.userID = userID
	f.historyIDArg = historyID
	return f.entry, f.getErr
}

func (f *fakeHistoryService) Delete(_ context.Context, userID, historyID uuid.UUID) error {
	f.calls++
	f.userID = userID
	f.historyIDArg = historyID
	return f.deleteErr
}

func newHistoryEngine(service *fakeHistoryService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.Logging(slog.New(slog.NewJSONHandler(io.Discard, nil))))

	handler := handlers.NewHistoryHandler(service)
	engine.GET("/history", middleware.Authenticate(fakeIssuer{userID: testUserID}), handler.List)
	engine.GET("/history/:id", middleware.Authenticate(fakeIssuer{userID: testUserID}), handler.Get)
	engine.DELETE("/history/:id", middleware.Authenticate(fakeIssuer{userID: testUserID}), handler.Delete)
	return engine
}

func TestHistoryListUsesDefaults(t *testing.T) {
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	service := &fakeHistoryService{page: models.HistoryPage{
		Items: []models.HistoryListItem{{
			ID:             uuid.New(),
			Query:          "космос",
			Filters:        models.Filters{GenreIDs: []int64{878}},
			RequestedLimit: 10,
			CreatedAt:      createdAt,
		}},
		Page:     1,
		PageSize: 20,
		Total:    1,
	}}
	engine := newHistoryEngine(service)

	recorder := testutils.Perform(t, engine, http.MethodGet, "/history", nil, bearerHeader())
	testutils.RequireStatus(t, recorder, http.StatusOK)

	if service.pageArg != 1 || service.pageSizeArg != 20 {
		t.Fatalf("service received page=%d size=%d", service.pageArg, service.pageSizeArg)
	}

	body := testutils.DecodeJSON[dto.HistoryListResponse](t, recorder)
	if body.Total != 1 || len(body.Items) != 1 || body.Items[0].Query != "космос" {
		t.Fatalf("response = %+v", body)
	}
	if len(body.Items[0].Filters.GenreIDs) != 1 || body.Items[0].Filters.GenreIDs[0] != 878 {
		t.Fatalf("filters = %+v", body.Items[0].Filters)
	}
}

func TestHistoryListAcceptsPagination(t *testing.T) {
	service := &fakeHistoryService{page: models.HistoryPage{Page: 2, PageSize: 50}}
	engine := newHistoryEngine(service)

	recorder := testutils.Perform(t, engine, http.MethodGet, "/history?page=2&page_size=50", nil, bearerHeader())
	testutils.RequireStatus(t, recorder, http.StatusOK)

	if service.pageArg != 2 || service.pageSizeArg != 50 {
		t.Fatalf("service received page=%d size=%d", service.pageArg, service.pageSizeArg)
	}
}

func TestHistoryListRejectsBadPagination(t *testing.T) {
	for _, target := range []string{"/history?page=abc", "/history?page=0", "/history?page_size=0", "/history?page_size=101"} {
		t.Run(target, func(t *testing.T) {
			service := &fakeHistoryService{}
			engine := newHistoryEngine(service)

			recorder := testutils.Perform(t, engine, http.MethodGet, target, nil, bearerHeader())
			testutils.RequireStatus(t, recorder, http.StatusUnprocessableEntity)

			body := testutils.DecodeJSON[dto.ErrorResponse](t, recorder)
			if body.Error.Code != dto.CodeValidationError {
				t.Fatalf("code = %q", body.Error.Code)
			}
			if service.calls != 0 {
				t.Fatal("invalid pagination must not reach the service")
			}
		})
	}
}

func TestHistoryGetReturnsSnapshot(t *testing.T) {
	historyID := uuid.New()
	year := int32(1999)
	service := &fakeHistoryService{entry: models.HistoryEntry{
		ID:             historyID,
		Query:          "космос",
		RequestedLimit: 10,
		Response: models.Recommendation{
			Mode:         "retrieval_only",
			StrategyUsed: "hybrid",
			Items: []models.RecommendedMovie{{
				Movie: models.MovieCard{MovieID: 603, Title: "The Matrix", Year: &year},
			}},
		},
		CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}}
	engine := newHistoryEngine(service)

	recorder := testutils.Perform(t, engine, http.MethodGet, "/history/"+historyID.String(), nil, bearerHeader())
	testutils.RequireStatus(t, recorder, http.StatusOK)

	if service.historyIDArg != historyID {
		t.Fatalf("service received id %v", service.historyIDArg)
	}

	body := testutils.DecodeJSON[dto.HistoryEntryResponse](t, recorder)
	if body.HistoryID != historyID || body.Response.Mode != "retrieval_only" {
		t.Fatalf("response = %+v", body)
	}
	if len(body.Response.Items) != 1 || body.Response.Items[0].MovieID != 603 {
		t.Fatalf("items = %+v", body.Response.Items)
	}
	if body.Response.Items[0].Reason != nil {
		t.Fatalf("retrieval_only item must have no reason: %v", body.Response.Items[0].Reason)
	}
}

func TestHistoryGetRejectsInvalidID(t *testing.T) {
	engine := newHistoryEngine(&fakeHistoryService{})

	recorder := testutils.Perform(t, engine, http.MethodGet, "/history/not-a-uuid", nil, bearerHeader())
	testutils.RequireStatus(t, recorder, http.StatusUnprocessableEntity)

	body := testutils.DecodeJSON[dto.ErrorResponse](t, recorder)
	if body.Error.Code != dto.CodeValidationError {
		t.Fatalf("code = %q", body.Error.Code)
	}
}

func TestHistoryGetMapsNotFound(t *testing.T) {
	engine := newHistoryEngine(&fakeHistoryService{getErr: rules.ErrNotFound})

	recorder := testutils.Perform(t, engine, http.MethodGet, "/history/"+uuid.NewString(), nil, bearerHeader())
	testutils.RequireStatus(t, recorder, http.StatusNotFound)

	body := testutils.DecodeJSON[dto.ErrorResponse](t, recorder)
	if body.Error.Code != dto.CodeNotFound {
		t.Fatalf("code = %q", body.Error.Code)
	}
}

func TestHistoryDeleteReturnsNoContent(t *testing.T) {
	historyID := uuid.New()
	service := &fakeHistoryService{}
	engine := newHistoryEngine(service)

	recorder := testutils.Perform(t, engine, http.MethodDelete, "/history/"+historyID.String(), nil, bearerHeader())
	testutils.RequireStatus(t, recorder, http.StatusNoContent)

	if service.calls != 1 || service.historyIDArg != historyID || service.userID != testUserID {
		t.Fatalf("service call = %+v", service)
	}
}

func TestHistoryDeleteMapsNotFound(t *testing.T) {
	engine := newHistoryEngine(&fakeHistoryService{deleteErr: rules.ErrNotFound})

	recorder := testutils.Perform(t, engine, http.MethodDelete, "/history/"+uuid.NewString(), nil, bearerHeader())
	testutils.RequireStatus(t, recorder, http.StatusNotFound)
}

func TestHistoryRequiresAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.Logging(slog.New(slog.NewJSONHandler(io.Discard, nil))))
	engine.GET("/history", handlers.NewHistoryHandler(&fakeHistoryService{}).List)

	recorder := testutils.Perform(t, engine, http.MethodGet, "/history", nil, bearerHeader())
	testutils.RequireStatus(t, recorder, http.StatusUnauthorized)
}
