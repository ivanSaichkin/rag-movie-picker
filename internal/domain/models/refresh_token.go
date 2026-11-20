package models

import (
	"time"

	"github.com/google/uuid"
)

// RefreshToken is a long-lived opaque token bound to one session. Only the hash
// is stored; the client keeps the original value in an HttpOnly cookie.
type RefreshToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash []byte
	FamilyID  uuid.UUID
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

// Revoked reports whether the token was already used or invalidated.
func (t RefreshToken) Revoked() bool {
	return t.RevokedAt != nil
}
