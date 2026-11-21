package catalog_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/services/catalog"
)

type fakeRAG struct {
	catalog     models.Catalog
	catalogErr  error
	catalogCall int
	movie       models.MovieCard
	movieErr    error
	movieCall   int
	movieID     int64
}

func (f *fakeRAG) Catalog(context.Context) (models.Catalog, error) {
	f.catalogCall++
	return f.catalog, f.catalogErr
}

func (f *fakeRAG) Movie(_ context.Context, movieID int64) (models.MovieCard, error) {
	f.movieCall++
	f.movieID = movieID
	return f.movie, f.movieErr
}

func (f *fakeRAG) Recommend(context.Context, models.RecommendationRequest) (models.Recommendation, error) {
	return models.Recommendation{}, errors.New("unexpected call")
}

type fakeCache struct {
	values  map[string][]byte
	getErr  error
	setErr  error
	gets    int
	sets    int
	lastKey string
	lastTTL time.Duration
}

func (f *fakeCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	f.gets++
	if f.getErr != nil {
		return nil, false, f.getErr
	}
	value, found := f.values[key]
	return value, found, nil
}

func (f *fakeCache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	f.sets++
	f.lastKey = key
	f.lastTTL = ttl
	if f.setErr != nil {
		return f.setErr
	}
	if f.values == nil {
		f.values = make(map[string][]byte)
	}
	f.values[key] = append([]byte(nil), value...)
	return nil
}

func (f *fakeCache) Increment(context.Context, string, time.Duration) (int64, error) {
	return 0, errors.New("unexpected call")
}

func (f *fakeCache) Delete(context.Context, string) error {
	return errors.New("unexpected call")
}

func sampleCatalog() models.Catalog {
	return models.Catalog{
		Genres:              []models.Genre{{ID: 878, Name: "Science Fiction"}},
		MovieCount:          4803,
		YearFrom:            int32Pointer(1902),
		YearTo:              int32Pointer(2017),
		DefaultResults:      10,
		MaxResults:          50,
		AvailableStrategies: []string{"bm25", "transformer", "hybrid"},
		DefaultStrategy:     "hybrid",
	}
}

func int32Pointer(value int32) *int32 {
	return &value
}

func newService(rag *fakeRAG, cache *fakeCache) *catalog.Service {
	return catalog.NewService(rag, cache, slog.New(slog.NewJSONHandler(io.Discard, nil)))
}

func TestCatalogFillsCacheAndServesFromIt(t *testing.T) {
	rag := &fakeRAG{catalog: sampleCatalog()}
	cache := &fakeCache{}
	service := newService(rag, cache)

	first, err := service.Catalog(context.Background())
	if err != nil {
		t.Fatalf("Catalog() error = %v", err)
	}
	if rag.catalogCall != 1 || cache.sets != 1 {
		t.Fatalf("calls: rag=%d, cache sets=%d", rag.catalogCall, cache.sets)
	}
	if cache.lastKey != "catalog:current" {
		t.Fatalf("cache key = %q", cache.lastKey)
	}
	if cache.lastTTL != 5*time.Minute {
		t.Fatalf("cache TTL = %v, want 5m", cache.lastTTL)
	}

	second, err := service.Catalog(context.Background())
	if err != nil {
		t.Fatalf("Catalog() error = %v", err)
	}
	if rag.catalogCall != 1 {
		t.Fatalf("second call must be served from cache, rag calls = %d", rag.catalogCall)
	}
	if len(second.Genres) != 1 || second.Genres[0].ID != 878 {
		t.Fatalf("cached genres = %+v", second.Genres)
	}
	if second.YearFrom == nil || *second.YearFrom != *first.YearFrom {
		t.Fatalf("cached bounds = %v", second.YearFrom)
	}
	if len(second.AvailableStrategies) != 3 || second.DefaultStrategy != "hybrid" {
		t.Fatalf("cached strategies = %+v", second)
	}
}

func TestCatalogRecoversFromCorruptedCache(t *testing.T) {
	rag := &fakeRAG{catalog: sampleCatalog()}
	cache := &fakeCache{values: map[string][]byte{"catalog:current": []byte("{not json")}}
	service := newService(rag, cache)

	if _, err := service.Catalog(context.Background()); err != nil {
		t.Fatalf("Catalog() error = %v", err)
	}
	if rag.catalogCall != 1 || cache.sets != 1 {
		t.Fatalf("calls: rag=%d, sets=%d", rag.catalogCall, cache.sets)
	}
}

func TestCatalogSurvivesCacheReadFailure(t *testing.T) {
	rag := &fakeRAG{catalog: sampleCatalog()}
	cache := &fakeCache{getErr: errors.New("redis is down")}
	service := newService(rag, cache)

	catalogResult, err := service.Catalog(context.Background())
	if err != nil {
		t.Fatalf("Catalog() error = %v", err)
	}
	if catalogResult.MovieCount != 4803 || rag.catalogCall != 1 {
		t.Fatalf("result = %+v", catalogResult)
	}
}

func TestCatalogSurvivesCacheWriteFailure(t *testing.T) {
	rag := &fakeRAG{catalog: sampleCatalog()}
	cache := &fakeCache{setErr: errors.New("redis is down")}
	service := newService(rag, cache)

	if _, err := service.Catalog(context.Background()); err != nil {
		t.Fatalf("Catalog() error = %v", err)
	}
}

func TestCatalogDoesNotCacheErrors(t *testing.T) {
	rag := &fakeRAG{catalogErr: errors.New("rag is down")}
	cache := &fakeCache{}
	service := newService(rag, cache)

	if _, err := service.Catalog(context.Background()); err == nil {
		t.Fatal("Catalog() must propagate the RAG error")
	}
	if cache.sets != 0 {
		t.Fatalf("failed catalog must not be cached, sets = %d", cache.sets)
	}
}

func TestMovieIsReadFromRAG(t *testing.T) {
	rag := &fakeRAG{movie: models.MovieCard{MovieID: 603, Title: "The Matrix"}}
	cache := &fakeCache{}
	service := newService(rag, cache)

	movie, err := service.Movie(context.Background(), 603)
	if err != nil {
		t.Fatalf("Movie() error = %v", err)
	}
	if movie.Title != "The Matrix" || rag.movieID != 603 {
		t.Fatalf("movie = %+v, requested id = %d", movie, rag.movieID)
	}
	if cache.gets != 0 || cache.sets != 0 {
		t.Fatalf("cards must not touch the catalog cache: gets=%d, sets=%d", cache.gets, cache.sets)
	}
}
