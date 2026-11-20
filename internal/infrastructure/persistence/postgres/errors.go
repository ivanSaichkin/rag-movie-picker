package postgres

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ivanSaichkin/rag-movie-picker/internal/domain/rules"
)

const (
	uniqueViolation = "23505"
	undefinedTable  = "42P01"
)

// mapError converts driver errors into domain errors, so that services do not
// depend on PostgreSQL error codes.
func mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return rules.ErrNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case uniqueViolation:
			return rules.ErrConflict
		case undefinedTable:
			return rules.ErrNotFound
		}
	}
	return err
}

// wrapQueryError maps driver errors to domain errors and adds query context to
// everything else.
func wrapQueryError(action string, err error) error {
	mapped := mapError(err)
	if errors.Is(mapped, rules.ErrNotFound) || errors.Is(mapped, rules.ErrConflict) {
		return mapped
	}
	return fmt.Errorf("%s: %w", action, err)
}
