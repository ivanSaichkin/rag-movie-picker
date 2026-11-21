// Package ports declares the contracts the application depends on.
package ports

import (
	"context"

	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
)

// UserMovieRepository stores the personal lists. Every method is scoped to the
// owner.
type UserMovieRepository interface {
	// Save inserts a movie or updates its status and rating. The snapshot of
	// an existing record is preserved.
	Save(ctx context.Context, movie models.UserMovie) (models.UserMovie, error)
	Find(ctx context.Context, userID uuid.UUID, movieID int64) (models.UserMovie, error)
	Delete(ctx context.Context, userID uuid.UUID, movieID int64) (bool, error)
	List(ctx context.Context, userID uuid.UUID, status models.UserMovieStatus, limit, offset int) ([]models.UserMovie, int64, error)
}

// UserMovieService serves the personal lists: want to watch and watched.
type UserMovieService interface {
	Set(ctx context.Context, userID uuid.UUID, movieID int64, update models.UserMovieUpdate) (models.UserMovie, error)
	Delete(ctx context.Context, userID uuid.UUID, movieID int64) error
	List(ctx context.Context, userID uuid.UUID, status models.UserMovieStatus, page, pageSize int) (models.UserMoviePage, error)
}
