// Package rules contains the domain errors and validation rules.
package rules

import "errors"

var (
	// ErrNotFound reports a missing record.
	ErrNotFound = errors.New("not found")
	// ErrConflict reports a uniqueness violation.
	ErrConflict = errors.New("conflict")
	// ErrEmailTaken reports a registration attempt with an existing email.
	ErrEmailTaken = errors.New("email already registered")
	// ErrInvalidCredentials hides whether the email or the password was wrong.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrUnauthenticated reports a missing, malformed or revoked credential.
	ErrUnauthenticated = errors.New("unauthenticated")
	// ErrTokenExpired reports an expired access token.
	ErrTokenExpired = errors.New("token expired")
)
