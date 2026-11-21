// Package catalog serves the RAG catalog through the cache and movie cards
// directly from the RAG service.
package catalog

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/ports"
)

const (
	cacheKey = "catalog:current"
	cacheTTL = 5 * time.Minute
)

var _ ports.CatalogService = (*Service)(nil)

// Service keeps the catalog in the cache for a few minutes: the index rarely
// changes, while the RAG call is expensive. Movie cards are read directly.
type Service struct {
	rag    ports.RAGClient
	cache  ports.Cache
	logger *slog.Logger
}

// NewService builds the catalog service.
func NewService(rag ports.RAGClient, cache ports.Cache, logger *slog.Logger) *Service {
	return &Service{rag: rag, cache: cache, logger: logger}
}

// Catalog returns the cached catalog or refills the cache from RAG.
func (s *Service) Catalog(ctx context.Context) (models.Catalog, error) {
	if catalog, found := s.fromCache(ctx); found {
		return catalog, nil
	}

	catalog, err := s.rag.Catalog(ctx)
	if err != nil {
		return models.Catalog{}, err
	}

	s.toCache(ctx, catalog)
	return catalog, nil
}

// Movie reads a card from RAG without caching: cards must not lag behind the
// index, and a single call is cheap.
func (s *Service) Movie(ctx context.Context, movieID int64) (models.MovieCard, error) {
	return s.rag.Movie(ctx, movieID)
}

// fromCache treats every cache problem as a miss: a broken cache must not break
// the catalog.
func (s *Service) fromCache(ctx context.Context) (models.Catalog, bool) {
	raw, found, err := s.cache.Get(ctx, cacheKey)
	if err != nil {
		s.logger.Warn("catalog cache read failed", "error", err)
		return models.Catalog{}, false
	}
	if !found {
		return models.Catalog{}, false
	}

	var entry cachedCatalog
	if err := json.Unmarshal(raw, &entry); err != nil {
		s.logger.Warn("catalog cache is corrupted", "error", err)
		return models.Catalog{}, false
	}
	return entry.toModel(), true
}

func (s *Service) toCache(ctx context.Context, catalog models.Catalog) {
	raw, err := json.Marshal(cachedCatalogFromModel(catalog))
	if err != nil {
		s.logger.Warn("catalog cache encoding failed", "error", err)
		return
	}
	if err := s.cache.Set(ctx, cacheKey, raw, cacheTTL); err != nil {
		s.logger.Warn("catalog cache write failed", "error", err)
	}
}

// cachedCatalog keeps the cache format independent of the domain structs.
type cachedCatalog struct {
	Genres              []cachedGenre `json:"genres"`
	MovieCount          int64         `json:"movie_count"`
	YearFrom            *int32        `json:"year_from,omitempty"`
	YearTo              *int32        `json:"year_to,omitempty"`
	DefaultResults      int           `json:"default_results"`
	MaxResults          int           `json:"max_results"`
	AvailableStrategies []string      `json:"available_strategies"`
	DefaultStrategy     string        `json:"default_strategy"`
}

type cachedGenre struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func cachedCatalogFromModel(catalog models.Catalog) cachedCatalog {
	genres := make([]cachedGenre, 0, len(catalog.Genres))
	for _, genre := range catalog.Genres {
		genres = append(genres, cachedGenre{ID: genre.ID, Name: genre.Name})
	}
	return cachedCatalog{
		Genres:              genres,
		MovieCount:          catalog.MovieCount,
		YearFrom:            catalog.YearFrom,
		YearTo:              catalog.YearTo,
		DefaultResults:      catalog.DefaultResults,
		MaxResults:          catalog.MaxResults,
		AvailableStrategies: append([]string(nil), catalog.AvailableStrategies...),
		DefaultStrategy:     catalog.DefaultStrategy,
	}
}

func (c cachedCatalog) toModel() models.Catalog {
	genres := make([]models.Genre, 0, len(c.Genres))
	for _, genre := range c.Genres {
		genres = append(genres, models.Genre{ID: genre.ID, Name: genre.Name})
	}
	return models.Catalog{
		Genres:              genres,
		MovieCount:          c.MovieCount,
		YearFrom:            c.YearFrom,
		YearTo:              c.YearTo,
		DefaultResults:      c.DefaultResults,
		MaxResults:          c.MaxResults,
		AvailableStrategies: append([]string(nil), c.AvailableStrategies...),
		DefaultStrategy:     c.DefaultStrategy,
	}
}
