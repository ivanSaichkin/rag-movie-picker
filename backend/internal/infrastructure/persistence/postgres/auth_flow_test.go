//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/adapters/password"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/adapters/token"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/persistence/postgres"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/services/auth"
)

const (
	flowPassword = "password123"
	flowSecret   = "0123456789abcdef0123456789abcdef"
)

func TestAuthFlowWithRealRepositories(t *testing.T) {
	_, pool := newTestDatabase(t)
	ctx := context.Background()

	// Cheap Argon2 parameters keep the test fast; the format is the same.
	hasher := password.New(1, 8, 1)
	issuer := token.New(flowSecret, time.Minute)
	service, err := auth.New(postgres.NewUsers(pool), postgres.NewRefreshTokens(pool), hasher, issuer, time.Hour)
	if err != nil {
		t.Fatalf("auth.New() error = %v", err)
	}

	session, err := service.Register(ctx, "  Flow@Example.com ", flowPassword)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if session.User.Email != "flow@example.com" {
		t.Fatalf("email = %q, want the normalized address", session.User.Email)
	}
	if session.AccessToken == "" || session.RefreshToken == "" {
		t.Fatal("both tokens must be issued")
	}

	userID, err := issuer.ParseAccess(session.AccessToken)
	if err != nil {
		t.Fatalf("ParseAccess() error = %v", err)
	}
	if userID != session.User.ID {
		t.Fatalf("access token subject = %v, want %v", userID, session.User.ID)
	}

	if _, err := service.Register(ctx, "flow@example.com", flowPassword); !errors.Is(err, rules.ErrEmailTaken) {
		t.Fatalf("duplicate registration error = %v, want ErrEmailTaken", err)
	}

	loggedIn, err := service.Login(ctx, "FLOW@example.com", flowPassword)
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if loggedIn.User.ID != session.User.ID {
		t.Fatalf("login user = %v, want %v", loggedIn.User.ID, session.User.ID)
	}

	if _, err := service.Login(ctx, "flow@example.com", "wrong-password"); !errors.Is(err, rules.ErrInvalidCredentials) {
		t.Fatalf("wrong password error = %v, want ErrInvalidCredentials", err)
	}
	if _, err := service.Login(ctx, "nobody@example.com", flowPassword); !errors.Is(err, rules.ErrInvalidCredentials) {
		t.Fatalf("unknown email error = %v, want ErrInvalidCredentials", err)
	}

	me, err := service.Me(ctx, session.User.ID)
	if err != nil {
		t.Fatalf("Me() error = %v", err)
	}
	if me.Email != "flow@example.com" {
		t.Fatalf("Me() email = %q", me.Email)
	}

	refreshed, err := service.Refresh(ctx, loggedIn.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if refreshed.RefreshToken == loggedIn.RefreshToken {
		t.Fatal("refresh must rotate the token")
	}
	if refreshed.User.ID != session.User.ID {
		t.Fatalf("refreshed user = %v", refreshed.User.ID)
	}

	// Reusing a rotated token revokes the whole family.
	if _, err := service.Refresh(ctx, loggedIn.RefreshToken); !errors.Is(err, rules.ErrUnauthenticated) {
		t.Fatalf("reused token error = %v, want ErrUnauthenticated", err)
	}
	if _, err := service.Refresh(ctx, refreshed.RefreshToken); !errors.Is(err, rules.ErrUnauthenticated) {
		t.Fatalf("sibling token after reuse error = %v, want ErrUnauthenticated", err)
	}

	fresh, err := service.Login(ctx, "flow@example.com", flowPassword)
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if err := service.Logout(ctx, fresh.RefreshToken); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if _, err := service.Refresh(ctx, fresh.RefreshToken); !errors.Is(err, rules.ErrUnauthenticated) {
		t.Fatalf("refresh after logout error = %v, want ErrUnauthenticated", err)
	}
	if err := service.Logout(ctx, "never-issued-token"); err != nil {
		t.Fatalf("Logout() of an unknown token error = %v, want nil", err)
	}
}
