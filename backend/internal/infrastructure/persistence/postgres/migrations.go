package postgres

import (
	"embed"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
	// The blank import registers the pgx5 driver scheme used below.
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

var migrationNamePattern = regexp.MustCompile(`^(\d+)_.*\.up\.sql$`)

// LatestVersion returns the highest migration version available in the binary.
// ReadinessCheck compares it with the version applied to the database.
func LatestVersion() (uint, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return 0, fmt.Errorf("read embedded migrations: %w", err)
	}

	var latest uint
	for _, entry := range entries {
		match := migrationNamePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		version, err := strconv.ParseUint(match[1], 10, 32)
		if err != nil {
			return 0, fmt.Errorf("parse migration version %q: %w", entry.Name(), err)
		}
		if uint(version) > latest {
			latest = uint(version)
		}
	}
	if latest == 0 {
		return 0, errors.New("no embedded migrations found")
	}
	return latest, nil
}

// Up applies all pending migrations.
func Up(databaseURL string) error {
	return withMigrator(databaseURL, func(migrator *migrate.Migrate) error {
		if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("apply migrations: %w", err)
		}
		return nil
	})
}

// Down rolls back steps migrations.
func Down(databaseURL string, steps int) error {
	if steps <= 0 {
		return fmt.Errorf("steps must be positive, got %d", steps)
	}
	return withMigrator(databaseURL, func(migrator *migrate.Migrate) error {
		if err := migrator.Steps(-steps); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("rollback migrations: %w", err)
		}
		return nil
	})
}

// Version returns the applied migration version and the dirty flag.
func Version(databaseURL string) (uint, bool, error) {
	var (
		version uint
		dirty   bool
	)
	err := withMigrator(databaseURL, func(migrator *migrate.Migrate) error {
		applied, isDirty, err := migrator.Version()
		if errors.Is(err, migrate.ErrNilVersion) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read schema version: %w", err)
		}
		version, dirty = applied, isDirty
		return nil
	})
	return version, dirty, err
}

func withMigrator(databaseURL string, action func(*migrate.Migrate) error) (err error) {
	migrator, err := newMigrator(databaseURL)
	if err != nil {
		return err
	}
	defer func() {
		sourceErr, databaseErr := migrator.Close()
		err = errors.Join(err, sourceErr, databaseErr)
	}()

	return action(migrator)
}

func newMigrator(databaseURL string) (*migrate.Migrate, error) {
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("create migration source: %w", err)
	}

	target, err := migrationURL(databaseURL)
	if err != nil {
		return nil, err
	}

	migrator, err := migrate.NewWithSourceInstance("iofs", source, target)
	if err != nil {
		return nil, fmt.Errorf("create migrator: %w", err)
	}
	return migrator, nil
}

// migrationURL rewrites the service DSN scheme to the one registered by the
// golang-migrate pgx driver, which opens its own connection.
func migrationURL(databaseURL string) (string, error) {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return "", fmt.Errorf("parse DATABASE_URL: %w", err)
	}

	switch parsed.Scheme {
	case "pgx5":
		return databaseURL, nil
	case "postgres", "postgresql":
		parsed.Scheme = "pgx5"
		return parsed.String(), nil
	default:
		return "", fmt.Errorf("DATABASE_URL: unsupported scheme %q, expected postgres or postgresql", parsed.Scheme)
	}
}
