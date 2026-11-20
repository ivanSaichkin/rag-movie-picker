// Package models contains the domain data structures.
package models

import (
	"time"

	"github.com/google/uuid"
)

// User is an account of the service.
type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}
