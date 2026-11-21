// Package ports declares the contracts the application depends on.
package ports

import (
	"context"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
)

// RAGClient is the adapter contract for the Python RAG service: catalog and
// card lookups plus recommendations.
type RAGClient interface {
	Catalog(ctx context.Context) (models.Catalog, error)
	Movie(ctx context.Context, movieID int64) (models.MovieCard, error)
	Recommend(ctx context.Context, request models.RecommendationRequest) (models.Recommendation, error)
}

// CatalogService exposes the catalog and movie cards to the HTTP handlers. The
// implementation adds caching on top of RAGClient.
type CatalogService interface {
	Catalog(ctx context.Context) (models.Catalog, error)
	Movie(ctx context.Context, movieID int64) (models.MovieCard, error)
}
