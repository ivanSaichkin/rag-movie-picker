package dto

import (
	"time"

	"github.com/google/uuid"
)

// AuthCredentials is the request body of registration and login.
type AuthCredentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// UserResponse is the public representation of an account.
type UserResponse struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// AuthResponse returns the account and a fresh access token. The refresh token
// travels only in an HttpOnly cookie.
type AuthResponse struct {
	User        UserResponse `json:"user"`
	AccessToken string       `json:"access_token"`
	ExpiresIn   int          `json:"expires_in"`
}

// RefreshResponse returns a new access token after refresh rotation.
type RefreshResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}
