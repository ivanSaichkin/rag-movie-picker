// Package ports declares the contracts the application depends on.
package ports

import (
	"context"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
)

// RAGClient is the adapter contract for the Python RAG service. It grows with
// the recommendation call in a later stage; the catalog and movie card are the
// lookups the HTTP API needs now.
type RAGClient interface {
	Catalog(ctx context.Context) (models.Catalog, error)
	Movie(ctx context.Context, movieID int64) (models.MovieCard, error)
}

// CatalogService exposes the catalog and movie cards to the HTTP handlers. The
// implementation adds caching on top of RAGClient.
type CatalogService interface {
	Catalog(ctx context.Context) (models.Catalog, error)
	Movie(ctx context.Context, movieID int64) (models.MovieCard, error)
}
