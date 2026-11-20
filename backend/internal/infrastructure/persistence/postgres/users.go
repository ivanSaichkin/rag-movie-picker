package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
)

// Users stores accounts in PostgreSQL.
type Users struct {
	pool *pgxpool.Pool
}

// NewUsers builds the user repository.
func NewUsers(pool *pgxpool.Pool) *Users {
	return &Users{pool: pool}
}

// Create inserts a user and returns the generated identifier and creation time.
func (r *Users) Create(ctx context.Context, user models.User) (models.User, error) {
	const query = `
		INSERT INTO users (email, password_hash)
		VALUES ($1, $2)
		RETURNING id, created_at`

	err := r.pool.QueryRow(ctx, query, user.Email, user.PasswordHash).Scan(&user.ID, &user.CreatedAt)
	if err != nil {
		if errors.Is(mapError(err), rules.ErrConflict) {
			return models.User{}, rules.ErrEmailTaken
		}
		return models.User{}, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

// ByEmail finds a user by a case-insensitive email match.
func (r *Users) ByEmail(ctx context.Context, email string) (models.User, error) {
	const query = `
		SELECT id, email, password_hash, created_at
		FROM users
		WHERE lower(email) = lower($1)`

	var user models.User
	err := r.pool.QueryRow(ctx, query, email).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.CreatedAt)
	if err != nil {
		return models.User{}, wrapQueryError("find user by email", err)
	}
	return user, nil
}

// ByID finds a user by identifier.
func (r *Users) ByID(ctx context.Context, id uuid.UUID) (models.User, error) {
	const query = `
		SELECT id, email, password_hash, created_at
		FROM users
		WHERE id = $1`

	var user models.User
	err := r.pool.QueryRow(ctx, query, id).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.CreatedAt)
	if err != nil {
		return models.User{}, wrapQueryError("find user by id", err)
	}
	return user, nil
}
