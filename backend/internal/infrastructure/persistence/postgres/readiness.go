package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type pinger interface {
	Ping(ctx context.Context) error
}

type schemaVersionReader interface {
	SchemaVersion(ctx context.Context) (uint, bool, error)
}

// Readiness verifies that PostgreSQL answers and that the schema is applied at
// the version embedded into the binary.
type Readiness struct {
	db     pinger
	schema schemaVersionReader
	latest uint
}

// NewReadiness builds the readiness check for PostgreSQL.
func NewReadiness(pool *pgxpool.Pool) (*Readiness, error) {
	latest, err := LatestVersion()
	if err != nil {
		return nil, err
	}
	return &Readiness{db: pool, schema: NewSchema(pool), latest: latest}, nil
}

// Name returns the check name reported by /ready.
func (r *Readiness) Name() string {
	return "postgres"
}

// Check reports connectivity, a clean and a current schema version.
func (r *Readiness) Check(ctx context.Context) error {
	if err := r.db.Ping(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}

	version, dirty, err := r.schema.SchemaVersion(ctx)
	if err != nil {
		return fmt.Errorf("schema version: %w", err)
	}
	if dirty {
		return fmt.Errorf("schema is dirty at version %d", version)
	}
	if version != r.latest {
		return fmt.Errorf("schema version %d, want %d", version, r.latest)
	}
	return nil
}
