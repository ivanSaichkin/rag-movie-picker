// Package ports declares the contracts the application depends on.
package ports

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
)

// AuthService implements registration, login, refresh rotation and logout.
type AuthService interface {
	Register(ctx context.Context, email, password string) (models.Session, error)
	Login(ctx context.Context, email, password string) (models.Session, error)
	Refresh(ctx context.Context, refreshToken string) (models.Session, error)
	Logout(ctx context.Context, refreshToken string) error
	Me(ctx context.Context, userID uuid.UUID) (models.User, error)
}

// PasswordHasher creates and verifies password hashes.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password, hash string) (bool, error)
}

// TokenIssuer issues and parses access tokens. ParseAccess returns
// rules.ErrTokenExpired for an expired token and rules.ErrUnauthenticated for
// any other invalid token.
type TokenIssuer interface {
	IssueAccess(userID uuid.UUID) (token string, expiresIn int, err error)
	ParseAccess(token string) (userID uuid.UUID, err error)
}

// Cache is a key-value store with expiry used for rate limits and lookups. It
// reports misses separately from errors, so that a broken dependency does not
// look like an empty value.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
	Delete(ctx context.Context, key string) error
}
