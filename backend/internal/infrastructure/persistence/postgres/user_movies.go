package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/ports"
)

// UserMovies stores the personal lists in PostgreSQL.
type UserMovies struct {
	pool *pgxpool.Pool
}

// NewUserMovieRepository builds the personal list repository.
func NewUserMovieRepository(pool *pgxpool.Pool) *UserMovies {
	return &UserMovies{pool: pool}
}

// Save inserts a movie or updates its status and rating. The snapshot of an
// existing record is preserved: the card is taken from RAG only on the first
// write.
func (r *UserMovies) Save(ctx context.Context, movie models.UserMovie) (models.UserMovie, error) {
	snapshot, err := encodeMovieSnapshot(movie.Movie)
	if err != nil {
		return models.UserMovie{}, err
	}

	const query = `
		INSERT INTO user_movies (user_id, movie_id, status, liked, movie_snapshot, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (user_id, movie_id) DO UPDATE
		SET status = EXCLUDED.status,
		    liked = EXCLUDED.liked,
		    updated_at = now()
		RETURNING status, liked, movie_snapshot, updated_at`

	var (
		status      string
		liked       *bool
		rawSnapshot []byte
		updatedAt   time.Time
	)
	err = r.pool.QueryRow(ctx, query, movie.UserID, movie.MovieID, string(movie.Status), movie.Liked, snapshot).
		Scan(&status, &liked, &rawSnapshot, &updatedAt)
	if err != nil {
		return models.UserMovie{}, fmt.Errorf("save user movie: %w", err)
	}

	card, err := decodeMovieSnapshot(rawSnapshot)
	if err != nil {
		return models.UserMovie{}, err
	}

	return models.UserMovie{
		UserID:    movie.UserID,
		MovieID:   movie.MovieID,
		Status:    models.UserMovieStatus(status),
		Liked:     liked,
		Movie:     card,
		UpdatedAt: updatedAt,
	}, nil
}

// Find returns one movie of the user's list.
func (r *UserMovies) Find(ctx context.Context, userID uuid.UUID, movieID int64) (models.UserMovie, error) {
	const query = `
		SELECT status, liked, movie_snapshot, updated_at
		FROM user_movies
		WHERE user_id = $1 AND movie_id = $2`

	var (
		status      string
		liked       *bool
		rawSnapshot []byte
		updatedAt   time.Time
	)
	err := r.pool.QueryRow(ctx, query, userID, movieID).Scan(&status, &liked, &rawSnapshot, &updatedAt)
	if err != nil {
		return models.UserMovie{}, wrapQueryError("find user movie", err)
	}

	card, err := decodeMovieSnapshot(rawSnapshot)
	if err != nil {
		return models.UserMovie{}, err
	}

	return models.UserMovie{
		UserID:    userID,
		MovieID:   movieID,
		Status:    models.UserMovieStatus(status),
		Liked:     liked,
		Movie:     card,
		UpdatedAt: updatedAt,
	}, nil
}

// Delete removes the movie from the list and reports whether it existed.
func (r *UserMovies) Delete(ctx context.Context, userID uuid.UUID, movieID int64) (bool, error) {
	const query = `DELETE FROM user_movies WHERE user_id = $1 AND movie_id = $2`

	tag, err := r.pool.Exec(ctx, query, userID, movieID)
	if err != nil {
		return false, fmt.Errorf("delete user movie: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// List returns one page of the user's list for the status and the total count.
func (r *UserMovies) List(ctx context.Context, userID uuid.UUID, status models.UserMovieStatus, limit, offset int) ([]models.UserMovie, int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM user_movies WHERE user_id = $1 AND status = $2`,
		userID, string(status),
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count user movies: %w", err)
	}

	const query = `
		SELECT movie_id, status, liked, movie_snapshot, updated_at
		FROM user_movies
		WHERE user_id = $1 AND status = $2
		ORDER BY updated_at DESC, movie_id
		LIMIT $3 OFFSET $4`

	rows, err := r.pool.Query(ctx, query, userID, string(status), limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list user movies: %w", err)
	}
	defer rows.Close()

	items := make([]models.UserMovie, 0, limit)
	for rows.Next() {
		var (
			movieID     int64
			rowStatus   string
			liked       *bool
			rawSnapshot []byte
			updatedAt   time.Time
		)
		if err := rows.Scan(&movieID, &rowStatus, &liked, &rawSnapshot, &updatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan user movie: %w", err)
		}

		card, err := decodeMovieSnapshot(rawSnapshot)
		if err != nil {
			return nil, 0, err
		}

		items = append(items, models.UserMovie{
			UserID:    userID,
			MovieID:   movieID,
			Status:    models.UserMovieStatus(rowStatus),
			Liked:     liked,
			Movie:     card,
			UpdatedAt: updatedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate user movies: %w", err)
	}
	return items, total, nil
}

var _ ports.UserMovieRepository = (*UserMovies)(nil)
