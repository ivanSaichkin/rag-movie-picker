package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
)

// MigrationState reads the state of the applied migrations.
type MigrationState struct {
	pool *pgxpool.Pool
}

// NewMigrationState builds the schema reader.
func NewMigrationState(pool *pgxpool.Pool) *MigrationState {
	return &MigrationState{pool: pool}
}

// SchemaVersion returns the applied version. An empty migration table means
// that no migration was applied yet, which is reported as version zero.
func (s *MigrationState) SchemaVersion(ctx context.Context) (uint, bool, error) {
	const query = `SELECT version, dirty FROM schema_migrations`

	var (
		version uint
		dirty   bool
	)
	err := s.pool.QueryRow(ctx, query).Scan(&version, &dirty)
	if err != nil {
		mapped := mapError(err)
		if errors.Is(mapped, rules.ErrNotFound) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("read schema version: %w", err)
	}
	return version, dirty, nil
}
