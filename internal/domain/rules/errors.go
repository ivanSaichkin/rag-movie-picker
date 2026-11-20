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
)
