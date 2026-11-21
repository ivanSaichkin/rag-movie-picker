// Package ports declares the contracts the application depends on.
package ports

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
)

// UserRepository stores accounts.
type UserRepository interface {
	Create(ctx context.Context, user models.User) (models.User, error)
	FindByEmail(ctx context.Context, email string) (models.User, error)
	FindByID(ctx context.Context, id uuid.UUID) (models.User, error)
}

// RefreshTokenRepository stores refresh tokens and their revocation state.
type RefreshTokenRepository interface {
	Create(ctx context.Context, token models.RefreshToken) (models.RefreshToken, error)
	FindByHash(ctx context.Context, hash []byte) (models.RefreshToken, error)
	Revoke(ctx context.Context, id uuid.UUID) (bool, error)
	RevokeFamily(ctx context.Context, familyID uuid.UUID) (int64, error)
	DeleteExpired(ctx context.Context, cutoff time.Time) (int64, error)
}

// HistoryRepository stores recommendation snapshots. Every method is scoped to
// the owner: another user's record is reported as missing.
type HistoryRepository interface {
	Create(ctx context.Context, entry models.HistoryEntry) (models.HistoryEntry, error)
	List(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.HistoryListItem, int64, error)
	Get(ctx context.Context, userID, historyID uuid.UUID) (models.HistoryEntry, error)
	Delete(ctx context.Context, userID, historyID uuid.UUID) (bool, error)
}
