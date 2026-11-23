package recommendation_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/services/recommendation"
)

var testUserID = uuid.MustParse("11111111-1111-1111-1111-111111111111")

type fakeCatalog struct {
	catalog models.Catalog
	err     error
	calls   int
}

func (f *fakeCatalog) Catalog(context.Context) (models.Catalog, error) {
	f.calls++
	return f.catalog, f.err
}

func (f *fakeCatalog) Movie(context.Context, int64) (models.MovieCard, error) {
	return models.MovieCard{}, errors.New("unexpected call")
}

type fakeRAG struct {
	response models.Recommendation
	err      error
	calls    int
	request  models.RecommendationRequest
}

func (f *fakeRAG) Recommend(_ context.Context, request models.RecommendationRequest) (models.Recommendation, error) {
	f.calls++
	f.request = request
	return f.response, f.err
}

func (f *fakeRAG) Catalog(context.Context) (models.Catalog, error) {
	return models.Catalog{}, errors.New("unexpected call")
}

func (f *fakeRAG) Movie(context.Context, int64) (models.MovieCard, error) {
	return models.MovieCard{}, errors.New("unexpected call")
}

type fakeHistory struct {
	id        uuid.UUID
	checkErr  error
	createErr error
	calls     int
	entry     models.HistoryEntry
}

func (f *fakeHistory) CheckChat(context.Context, uuid.UUID, uuid.UUID) error { return f.checkErr }

func (f *fakeHistory) Create(_ context.Context, entry models.HistoryEntry) (models.HistoryEntry, error) {
	f.calls++
	f.entry = entry
	if f.createErr != nil {
		return models.HistoryEntry{}, f.createErr
	}
	entry.ID = f.id
	entry.CreatedAt = time.Now()
	return entry, nil
}

func (f *fakeHistory) List(context.Context, uuid.UUID, int, int) ([]models.HistoryListItem, int64, error) {
	return nil, 0, errors.New("unexpected call")
}

func (f *fakeHistory) Get(context.Context, uuid.UUID, uuid.UUID) (models.HistoryEntry, error) {
	return models.HistoryEntry{}, errors.New("unexpected call")
}

func (f *fakeHistory) Delete(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return false, errors.New("unexpected call")
}

func catalogFixture() models.Catalog {
	return models.Catalog{
		Genres:              []models.Genre{{ID: 878, Name: "Science Fiction"}},
		MovieCount:          4803,
		DefaultResults:      10,
		MaxResults:          50,
		AvailableStrategies: []string{"bm25", "transformer", "hybrid"},
		DefaultStrategy:     "hybrid",
	}
}

func newService(catalog *fakeCatalog, rag *fakeRAG, history *fakeHistory) *recommendation.Service {
	return recommendation.NewService(catalog, rag, history, slog.New(slog.NewJSONHandler(io.Discard, nil)))
}

func sampleResponse() models.Recommendation {
	reason := "подходит"
	return models.Recommendation{
		Mode:         "generated",
		StrategyUsed: "hybrid",
		Items:        []models.RecommendedMovie{{Movie: models.MovieCard{MovieID: 603, Title: "The Matrix"}, Reason: &reason}},
		Warnings:     []models.Warning{{Code: "FEWER_RESULTS", Message: "меньше"}},
	}
}

func TestRecommendResolvesLimitAndStoresHistory(t *testing.T) {
	historyID := uuid.New()
	catalog := &fakeCatalog{catalog: catalogFixture()}
	rag := &fakeRAG{response: sampleResponse()}
	history := &fakeHistory{id: historyID}
	service := newService(catalog, rag, history)

	result, err := service.Recommend(context.Background(), testUserID, models.RecommendationRequest{Query: "  космос  "})
	if err != nil {
		t.Fatalf("Recommend() error = %v", err)
	}

	if rag.calls != 1 {
		t.Fatalf("RAG calls = %d", rag.calls)
	}
	if rag.request.Query != "космос" {
		t.Fatalf("query passed = %q", rag.request.Query)
	}
	if rag.request.Limit == nil || *rag.request.Limit != 10 {
		t.Fatalf("resolved limit = %v", rag.request.Limit)
	}

	if history.calls != 1 {
		t.Fatalf("history calls = %d", history.calls)
	}
	if history.entry.UserID != testUserID || history.entry.Query != "космос" || history.entry.RequestedLimit != 10 {
		t.Fatalf("history entry = %+v", history.entry)
	}
	if len(history.entry.Response.Items) != 1 {
		t.Fatalf("stored response = %+v", history.entry.Response)
	}

	if result.HistoryID == nil || *result.HistoryID != historyID {
		t.Fatalf("history id = %v", result.HistoryID)
	}
	if result.Response.Mode != "generated" || len(result.Response.Items) != 1 {
		t.Fatalf("result = %+v", result.Response)
	}
}

func TestRecommendGuestForcesRetrievalAndSkipsHistory(t *testing.T) {
	enable := true
	rag := &fakeRAG{response: models.Recommendation{Mode: "retrieval_only"}}
	history := &fakeHistory{}
	service := newService(&fakeCatalog{catalog: catalogFixture()}, rag, history)
	result, err := service.RecommendGuest(t.Context(), models.RecommendationRequest{
		Query: " space ", DisableLLM: false, GenerateExplanations: &enable,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := rag.request
	if rag.calls != 1 || !request.DisableLLM || request.GenerateExplanations == nil || *request.GenerateExplanations {
		t.Fatalf("unexpected guest RAG request: %+v", request)
	}
	if request.Query != "space" || request.Limit == nil || *request.Limit != 10 {
		t.Fatalf("unresolved guest query or limit: %+v", request)
	}
	if result.Response.Mode != "retrieval_only" || result.HistoryID != nil || history.calls != 0 {
		t.Fatalf("guest result = %+v; history writes = %d", result, history.calls)
	}
}

func TestRecommendGuestRejectsSavedChatBeforeSearch(t *testing.T) {
	catalog, rag, history := &fakeCatalog{}, &fakeRAG{}, &fakeHistory{}
	service := newService(catalog, rag, history)
	chatID := uuid.New()
	_, err := service.RecommendGuest(t.Context(), models.RecommendationRequest{Query: "space", ChatID: &chatID})
	var validationErr *rules.ValidationError
	if !errors.As(err, &validationErr) || catalog.calls != 0 || rag.calls != 0 || history.calls != 0 {
		t.Fatalf("guest chat request: err=%v, catalog=%d, rag=%d, history=%d", err, catalog.calls, rag.calls, history.calls)
	}
}

func TestRecommendPassesFiltersLimitAndStrategy(t *testing.T) {
	limit := 25
	yearFrom, yearTo := int32(2000), int32(2015)
	rating := 6.5
	strategy := "transformer"

	rag := &fakeRAG{response: sampleResponse()}
	service := newService(&fakeCatalog{catalog: catalogFixture()}, rag, &fakeHistory{id: uuid.New()})

	request := models.RecommendationRequest{
		Query: "космос",
		Limit: &limit,
		Filters: models.Filters{
			GenreIDs:  []int64{878},
			YearFrom:  &yearFrom,
			YearTo:    &yearTo,
			MinRating: &rating,
		},
		Strategy: &strategy,
	}
	if _, err := service.Recommend(context.Background(), testUserID, request); err != nil {
		t.Fatalf("Recommend() error = %v", err)
	}

	if rag.request.Limit == nil || *rag.request.Limit != limit {
		t.Fatalf("limit passed = %v", rag.request.Limit)
	}
	if rag.request.Filters.GenreIDs[0] != 878 || *rag.request.Filters.MinRating != rating {
		t.Fatalf("filters passed = %+v", rag.request.Filters)
	}
	if rag.request.Strategy == nil || *rag.request.Strategy != strategy {
		t.Fatalf("strategy passed = %v", rag.request.Strategy)
	}
}

func TestRecommendValidatesRequest(t *testing.T) {
	limitZero, limitBig := 0, 51
	unknownGenre := int64(999)
	badRating := 11.0
	year := int32(1700)
	reversedFrom, reversedTo := int32(2015), int32(2000)
	unknownStrategy := "magic"

	cases := map[string]models.RecommendationRequest{
		"empty query":      {Query: "   "},
		"too long query":   {Query: strings.Repeat("a", rules.MaxQueryLength+1)},
		"zero limit":       {Query: "космос", Limit: &limitZero},
		"too big limit":    {Query: "космос", Limit: &limitBig},
		"unknown genre":    {Query: "космос", Filters: models.Filters{GenreIDs: []int64{unknownGenre}}},
		"bad rating":       {Query: "космос", Filters: models.Filters{MinRating: &badRating}},
		"bad year":         {Query: "космос", Filters: models.Filters{YearFrom: &year}},
		"reversed years":   {Query: "космос", Filters: models.Filters{YearFrom: &reversedFrom, YearTo: &reversedTo}},
		"unknown strategy": {Query: "космос", Strategy: &unknownStrategy},
	}

	for name, request := range cases {
		t.Run(name, func(t *testing.T) {
			rag := &fakeRAG{response: sampleResponse()}
			history := &fakeHistory{id: uuid.New()}
			service := newService(&fakeCatalog{catalog: catalogFixture()}, rag, history)

			_, err := service.Recommend(context.Background(), testUserID, request)
			var validationErr *rules.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %v, want a validation error", err)
			}
			if rag.calls != 0 || history.calls != 0 {
				t.Fatalf("invalid request must not touch RAG or history: rag=%d, history=%d", rag.calls, history.calls)
			}
		})
	}
}

func TestRecommendPropagatesCatalogFailure(t *testing.T) {
	rag := &fakeRAG{response: sampleResponse()}
	catalogErr := errors.New("catalog is down")
	service := newService(&fakeCatalog{err: catalogErr}, rag, &fakeHistory{id: uuid.New()})

	_, err := service.Recommend(context.Background(), testUserID, models.RecommendationRequest{Query: "космос"})
	if !errors.Is(err, catalogErr) {
		t.Fatalf("error = %v, want the catalog error", err)
	}
	if rag.calls != 0 {
		t.Fatal("RAG must not be called when the catalog failed")
	}
}

func TestRecommendPropagatesRAGFailure(t *testing.T) {
	ragErr := errors.New("rag is down")
	history := &fakeHistory{id: uuid.New()}
	service := newService(&fakeCatalog{catalog: catalogFixture()}, &fakeRAG{err: ragErr}, history)

	_, err := service.Recommend(context.Background(), testUserID, models.RecommendationRequest{Query: "космос"})
	if !errors.Is(err, ragErr) {
		t.Fatalf("error = %v, want the RAG error", err)
	}
	if history.calls != 0 {
		t.Fatal("failed search must not be stored")
	}
}

func TestRecommendReturnsResultWhenHistoryFails(t *testing.T) {
	history := &fakeHistory{createErr: errors.New("database is down")}
	service := newService(&fakeCatalog{catalog: catalogFixture()}, &fakeRAG{response: sampleResponse()}, history)

	result, err := service.Recommend(context.Background(), testUserID, models.RecommendationRequest{Query: "космос"})
	if err != nil {
		t.Fatalf("Recommend() error = %v", err)
	}
	if result.HistoryID != nil {
		t.Fatalf("history id = %v, want nil", result.HistoryID)
	}

	var found bool
	for _, warning := range result.Response.Warnings {
		if warning.Code == models.WarningHistoryNotSaved {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings = %+v, want %s", result.Response.Warnings, models.WarningHistoryNotSaved)
	}
	if result.Response.Mode != "generated" || len(result.Response.Items) != 1 {
		t.Fatalf("result must keep the search answer: %+v", result.Response)
	}
}

func TestRecommendStoresEmptyAnswer(t *testing.T) {
	history := &fakeHistory{id: uuid.New()}
	service := newService(
		&fakeCatalog{catalog: catalogFixture()},
		&fakeRAG{response: models.Recommendation{Mode: "empty"}},
		history,
	)

	result, err := service.Recommend(context.Background(), testUserID, models.RecommendationRequest{Query: "космос"})
	if err != nil {
		t.Fatalf("Recommend() error = %v", err)
	}
	if history.calls != 1 || result.HistoryID == nil {
		t.Fatalf("empty answer must be stored: calls=%d, id=%v", history.calls, result.HistoryID)
	}
}

func TestChatCheckedBeforeRetrieval(t *testing.T) {
	rag := &fakeRAG{}
	history := &fakeHistory{checkErr: rules.ErrNotFound}
	service := newService(&fakeCatalog{catalog: catalogFixture()}, rag, history)
	id := uuid.New()
	request := models.RecommendationRequest{Query: "космос", ChatID: &id}
	_, err := service.Recommend(context.Background(), testUserID, request)
	if !errors.Is(err, rules.ErrNotFound) || rag.calls != 0 || history.calls != 0 {
		t.Fatalf("missing chat err=%v, RAG calls=%d, saves=%d", err, rag.calls, history.calls)
	}
	_, err = service.RecommendGuest(context.Background(), request)
	var validation *rules.ValidationError
	if !errors.As(err, &validation) || rag.calls != 0 {
		t.Fatalf("guest chat err=%v, RAG calls=%d", err, rag.calls)
	}
	history.checkErr = nil
	result, err := service.Recommend(context.Background(), testUserID, request)
	if err != nil || result.ChatID == nil || *result.ChatID != id {
		t.Fatalf("append chat result=%+v, err=%v", result, err)
	}
}
