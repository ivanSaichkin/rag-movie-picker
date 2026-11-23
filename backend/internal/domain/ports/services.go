// Package ports declares the contracts the application depends on.
package ports

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
)

// AuthenticationService implements registration, login, refresh rotation and logout.
type AuthenticationService interface {
	Register(ctx context.Context, email, password string) (models.Session, error)
	Login(ctx context.Context, email, password string) (models.Session, error)
	Refresh(ctx context.Context, refreshToken string) (models.Session, error)
	Logout(ctx context.Context, refreshToken string) error
	CurrentUser(ctx context.Context, userID uuid.UUID) (models.User, error)
}

// PasswordHasher creates and verifies password hashes.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password, hash string) (bool, error)
}

// AccessTokenIssuer issues and parses access tokens. ParseAccessToken returns
// rules.ErrTokenExpired for an expired token and rules.ErrUnauthenticated for
// any other invalid token.
type AccessTokenIssuer interface {
	IssueAccessToken(userID uuid.UUID) (token string, expiresIn int, err error)
	ParseAccessToken(token string) (userID uuid.UUID, err error)
}

// Cache is a key-value store with expiry used for rate limits and lookups. It
// reports misses separately from errors, so that a broken dependency does not
// look like an empty value.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Increment(ctx context.Context, key string, ttl time.Duration) (int64, error)
	Delete(ctx context.Context, key string) error
}

// RecommendationService validates a search request, calls RAG and stores the
// result snapshot in the history.
type RecommendationService interface {
	Recommend(ctx context.Context, userID uuid.UUID, request models.RecommendationRequest) (models.RecommendationResult, error)
	RecommendGuest(ctx context.Context, request models.RecommendationRequest) (models.RecommendationResult, error)
}

// HistoryService serves the stored search history of one user.
type HistoryService interface {
	List(ctx context.Context, userID uuid.UUID, page, pageSize int) (models.HistoryPage, error)
	Get(ctx context.Context, userID, historyID uuid.UUID) (models.HistoryEntry, error)
	Delete(ctx context.Context, userID, historyID uuid.UUID) error
}
