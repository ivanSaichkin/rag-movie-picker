//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/persistence/postgres"
)

var adminDSN string

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := tcpostgres.Run(
		ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("app"),
		tcpostgres.WithUsername("app"),
		tcpostgres.WithPassword("app"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start postgres container: %v\n", err)
		os.Exit(1)
	}

	adminDSN, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "read connection string: %v\n", err)
		_ = container.Terminate(ctx)
		os.Exit(1)
	}

	code := m.Run()

	if err := container.Terminate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "terminate postgres container: %v\n", err)
	}
	os.Exit(code)
}

func TestMigrationsApplyAndRollback(t *testing.T) {
	dsn, pool := newTestDatabase(t)
	ctx := context.Background()

	version, dirty, err := postgres.Version(dsn)
	if err != nil {
		t.Fatalf("Version() error = %v", err)
	}
	latest, err := postgres.LatestVersion()
	if err != nil {
		t.Fatalf("LatestVersion() error = %v", err)
	}
	if version != latest || dirty {
		t.Fatalf("version = %d dirty = %t, want %d and clean", version, dirty, latest)
	}

	for _, table := range []string{"users", "refresh_tokens", "search_history", "user_movies"} {
		if !tableExists(t, ctx, pool, table) {
			t.Fatalf("table %q is missing after up", table)
		}
	}

	if err := postgres.Down(dsn, 1); err != nil {
		t.Fatalf("Down() error = %v", err)
	}

	version, dirty, err = postgres.Version(dsn)
	if err != nil {
		t.Fatalf("Version() after down error = %v", err)
	}
	if version != 0 || dirty {
		t.Fatalf("version = %d dirty = %t after down, want 0 and clean", version, dirty)
	}
	if tableExists(t, ctx, pool, "users") {
		t.Fatal("users table must be dropped on down")
	}
}

func TestReadinessRequiresCurrentSchema(t *testing.T) {
	dsn, pool := newTestDatabase(t)
	ctx := context.Background()

	readiness, err := postgres.NewReadinessCheck(pool)
	if err != nil {
		t.Fatalf("NewReadinessCheck() error = %v", err)
	}
	if err := readiness.Check(ctx); err != nil {
		t.Fatalf("readiness must pass on a migrated database: %v", err)
	}

	if err := postgres.Down(dsn, 1); err != nil {
		t.Fatalf("Down() error = %v", err)
	}

	err = readiness.Check(ctx)
	if err == nil {
		t.Fatal("readiness must fail when migrations are absent")
	}
	if !strings.Contains(err.Error(), "schema version 0") {
		t.Fatalf("error %q must report the applied version", err)
	}
}

func TestUsers(t *testing.T) {
	_, pool := newTestDatabase(t)
	ctx := context.Background()
	users := postgres.NewUserRepository(pool)

	created, err := users.Create(ctx, models.User{Email: "User@Example.com", PasswordHash: "hash"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID == uuid.Nil {
		t.Fatal("created user must have an id")
	}
	if created.CreatedAt.IsZero() {
		t.Fatal("created user must have created_at")
	}

	_, err = users.Create(ctx, models.User{Email: "user@example.com", PasswordHash: "other"})
	if !errors.Is(err, rules.ErrEmailTaken) {
		t.Fatalf("duplicate email error = %v, want ErrEmailTaken", err)
	}

	found, err := users.FindByEmail(ctx, "USER@EXAMPLE.COM")
	if err != nil {
		t.Fatalf("FindByEmail() error = %v", err)
	}
	if found.ID != created.ID || found.PasswordHash != "hash" {
		t.Fatalf("FindByEmail() = %+v", found)
	}

	byID, err := users.FindByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if byID.Email != "User@Example.com" {
		t.Fatalf("FindByID() email = %q", byID.Email)
	}

	if _, err := users.FindByID(ctx, uuid.New()); !errors.Is(err, rules.ErrNotFound) {
		t.Fatalf("FindByID(unknown) error = %v, want ErrNotFound", err)
	}
	if _, err := users.FindByEmail(ctx, "nobody@example.com"); !errors.Is(err, rules.ErrNotFound) {
		t.Fatalf("FindByEmail(unknown) error = %v, want ErrNotFound", err)
	}
}

func TestRefreshTokens(t *testing.T) {
	_, pool := newTestDatabase(t)
	ctx := context.Background()

	users := postgres.NewUserRepository(pool)
	user, err := users.Create(ctx, models.User{Email: "tokens@example.com", PasswordHash: "hash"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	tokens := postgres.NewRefreshTokenRepository(pool)
	family := uuid.New()
	expiresAt := time.Now().Add(time.Hour)

	first, err := tokens.Create(ctx, models.RefreshToken{
		UserID:    user.ID,
		TokenHash: []byte("hash-1"),
		FamilyID:  family,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	if first.ID == uuid.Nil || first.CreatedAt.IsZero() {
		t.Fatalf("created token = %+v", first)
	}

	second, err := tokens.Create(ctx, models.RefreshToken{
		UserID:    user.ID,
		TokenHash: []byte("hash-2"),
		FamilyID:  family,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("Create(second) error = %v", err)
	}

	found, err := tokens.FindByHash(ctx, []byte("hash-1"))
	if err != nil {
		t.Fatalf("FindByHash() error = %v", err)
	}
	if found.ID != first.ID || found.IsRevoked() {
		t.Fatalf("FindByHash() = %+v", found)
	}

	revoked, err := tokens.Revoke(ctx, first.ID)
	if err != nil || !revoked {
		t.Fatalf("Revoke() = %t, %v; want true, nil", revoked, err)
	}
	revoked, err = tokens.Revoke(ctx, first.ID)
	if err != nil || revoked {
		t.Fatalf("second Revoke() = %t, %v; want false, nil", revoked, err)
	}

	found, err = tokens.FindByHash(ctx, []byte("hash-1"))
	if err != nil {
		t.Fatalf("FindByHash() after revoke error = %v", err)
	}
	if !found.IsRevoked() {
		t.Fatal("revoked token must report the revocation time")
	}

	affected, err := tokens.RevokeFamily(ctx, family)
	if err != nil {
		t.Fatalf("RevokeFamily() error = %v", err)
	}
	if affected != 1 {
		t.Fatalf("RevokeFamily() = %d, want 1 active token", affected)
	}

	second, err = tokens.FindByHash(ctx, []byte("hash-2"))
	if err != nil {
		t.Fatalf("FindByHash(second) error = %v", err)
	}
	if !second.IsRevoked() {
		t.Fatal("family revocation must revoke the sibling token")
	}

	removed, err := tokens.DeleteExpired(ctx, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("DeleteExpired() error = %v", err)
	}
	if removed != 2 {
		t.Fatalf("DeleteExpired() = %d, want 2", removed)
	}

	if _, err := tokens.FindByHash(ctx, []byte("hash-1")); !errors.Is(err, rules.ErrNotFound) {
		t.Fatalf("FindByHash() after cleanup error = %v, want ErrNotFound", err)
	}
}

func TestUserMoviesConstraints(t *testing.T) {
	_, pool := newTestDatabase(t)
	ctx := context.Background()

	user, err := postgres.NewUserRepository(pool).Create(ctx, models.User{Email: "movies@example.com", PasswordHash: "hash"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	const insert = `
		INSERT INTO user_movies (user_id, movie_id, status, liked, movie_snapshot)
		VALUES ($1, $2, $3, $4, $5)`
	const snapshot = `{"title": "Alien"}`

	cases := []struct {
		name   string
		movie  int64
		status string
		liked  any
		body   string
	}{
		{name: "liked without watched", movie: 1, status: "want_to_watch", liked: true, body: snapshot},
		{name: "unknown status", movie: 1, status: "maybe", liked: nil, body: snapshot},
		{name: "snapshot is an array", movie: 1, status: "want_to_watch", liked: nil, body: `[]`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, insert, user.ID, testCase.movie, testCase.status, testCase.liked, testCase.body); err == nil {
				t.Fatal("insert must be rejected by a database constraint")
			}
		})
	}

	if _, err := pool.Exec(ctx, insert, user.ID, 2, "want_to_watch", nil, snapshot); err != nil {
		t.Fatalf("valid insert error = %v", err)
	}
	if _, err := pool.Exec(ctx, insert, user.ID, 2, "watched", nil, snapshot); err == nil {
		t.Fatal("duplicate user and movie pair must be rejected")
	}
}

func TestSearchHistoryConstraints(t *testing.T) {
	_, pool := newTestDatabase(t)
	ctx := context.Background()

	user, err := postgres.NewUserRepository(pool).Create(ctx, models.User{Email: "history@example.com", PasswordHash: "hash"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	const insert = `
		INSERT INTO search_history (user_id, query, filters, requested_limit, response)
		VALUES ($1, $2, $3, $4, $5)`

	cases := []struct {
		name    string
		query   string
		filters string
		limit   int
		body    string
	}{
		{name: "blank query", query: "   ", filters: `{}`, limit: 10, body: `{}`},
		{name: "filters are an array", query: "space", filters: `[]`, limit: 10, body: `{}`},
		{name: "response is an array", query: "space", filters: `{}`, limit: 10, body: `[]`},
		{name: "zero limit", query: "space", filters: `{}`, limit: 0, body: `{}`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := pool.Exec(ctx, insert, user.ID, testCase.query, testCase.filters, testCase.limit, testCase.body)
			if err == nil {
				t.Fatal("insert must be rejected by a database constraint")
			}
		})
	}

	if _, err := pool.Exec(ctx, insert, user.ID, "space adventure", `{"genre_ids": [878]}`, 10, `{"mode": "empty"}`); err != nil {
		t.Fatalf("valid insert error = %v", err)
	}
}

func TestNewPoolFailsWhenDatabaseIsUnreachable(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = postgres.NewPool(ctx, "postgres://app:app@"+address+"/app?sslmode=disable", 1)
	if err == nil {
		t.Fatal("NewPool() must fail when the database is unreachable")
	}
}

func newTestDatabase(t *testing.T) (string, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("connect as admin: %v", err)
	}

	name := "test_" + uuid.NewString()[:8]
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	if err := admin.Close(ctx); err != nil {
		t.Fatalf("close admin connection: %v", err)
	}

	dsn, err := databaseURL(adminDSN, name)
	if err != nil {
		t.Fatalf("rewrite database name: %v", err)
	}
	if err := postgres.Up(dsn); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	pool, err := postgres.NewPool(ctx, dsn, 5)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()

		dropCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		connection, err := pgx.Connect(dropCtx, adminDSN)
		if err != nil {
			t.Logf("connect for drop: %v", err)
			return
		}
		defer func() {
			if err := connection.Close(dropCtx); err != nil {
				t.Logf("close drop connection: %v", err)
			}
		}()
		if _, err := connection.Exec(dropCtx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Logf("drop test database: %v", err)
		}
	})

	return dsn, pool
}

func databaseURL(dsn, database string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parse DSN: %w", err)
	}
	parsed.Path = "/" + database
	return parsed.String(), nil
}

func tableExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table string) bool {
	t.Helper()

	const query = `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = $1
		)`

	var exists bool
	if err := pool.QueryRow(ctx, query, table).Scan(&exists); err != nil {
		t.Fatalf("check table %q: %v", table, err)
	}
	return exists
}
