package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
)

// RefreshTokens stores refresh tokens in PostgreSQL.
type RefreshTokens struct {
	pool *pgxpool.Pool
}

// NewRefreshTokens builds the refresh token repository.
func NewRefreshTokens(pool *pgxpool.Pool) *RefreshTokens {
	return &RefreshTokens{pool: pool}
}

// Create inserts a refresh token and returns the generated identifier and
// creation time.
func (r *RefreshTokens) Create(ctx context.Context, token models.RefreshToken) (models.RefreshToken, error) {
	const query = `
		INSERT INTO refresh_tokens (user_id, token_hash, family_id, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`

	err := r.pool.QueryRow(ctx, query, token.UserID, token.TokenHash, token.FamilyID, token.ExpiresAt).
		Scan(&token.ID, &token.CreatedAt)
	if err != nil {
		mapped := mapError(err)
		if errors.Is(mapped, rules.ErrConflict) {
			return models.RefreshToken{}, mapped
		}
		return models.RefreshToken{}, fmt.Errorf("create refresh token: %w", err)
	}
	return token, nil
}

// ByHash finds a refresh token by the hash stored in the database.
func (r *RefreshTokens) ByHash(ctx context.Context, hash []byte) (models.RefreshToken, error) {
	const query = `
		SELECT id, user_id, token_hash, family_id, expires_at, revoked_at, created_at
		FROM refresh_tokens
		WHERE token_hash = $1`

	var token models.RefreshToken
	err := r.pool.QueryRow(ctx, query, hash).Scan(
		&token.ID,
		&token.UserID,
		&token.TokenHash,
		&token.FamilyID,
		&token.ExpiresAt,
		&token.RevokedAt,
		&token.CreatedAt,
	)
	if err != nil {
		return models.RefreshToken{}, wrapQueryError("find refresh token", err)
	}
	return token, nil
}

// Revoke marks one token as revoked. It reports whether the token was active.
func (r *RefreshTokens) Revoke(ctx context.Context, id uuid.UUID) (bool, error) {
	const query = `
		UPDATE refresh_tokens
		SET revoked_at = now()
		WHERE id = $1 AND revoked_at IS NULL`

	tag, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return false, fmt.Errorf("revoke refresh token: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// RevokeFamily revokes every active token of a rotation family. It reports how
// many tokens were revoked.
func (r *RefreshTokens) RevokeFamily(ctx context.Context, familyID uuid.UUID) (int64, error) {
	const query = `
		UPDATE refresh_tokens
		SET revoked_at = now()
		WHERE family_id = $1 AND revoked_at IS NULL`

	tag, err := r.pool.Exec(ctx, query, familyID)
	if err != nil {
		return 0, fmt.Errorf("revoke refresh token family: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteStale removes tokens whose expiry or revocation is older than cutoff.
func (r *RefreshTokens) DeleteStale(ctx context.Context, cutoff time.Time) (int64, error) {
	const query = `
		DELETE FROM refresh_tokens
		WHERE expires_at < $1 OR revoked_at < $1`

	tag, err := r.pool.Exec(ctx, query, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete stale refresh tokens: %w", err)
	}
	return tag.RowsAffected(), nil
}
