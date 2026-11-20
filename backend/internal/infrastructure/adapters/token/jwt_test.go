package token_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/adapters/token"
)

const (
	testSecret     = "0123456789abcdef0123456789abcdef"
	anotherSecret  = "fedcba9876543210fedcba9876543210"
	accessTokenTTL = time.Minute
)

func TestIssueAndParse(t *testing.T) {
	issuer := token.New(testSecret, accessTokenTTL)
	userID := uuid.New()

	raw, expiresIn, err := issuer.IssueAccess(userID)
	if err != nil {
		t.Fatalf("IssueAccess() error = %v", err)
	}
	if expiresIn != 60 {
		t.Fatalf("expiresIn = %d, want 60", expiresIn)
	}

	parsed, err := issuer.ParseAccess(raw)
	if err != nil {
		t.Fatalf("ParseAccess() error = %v", err)
	}
	if parsed != userID {
		t.Fatalf("ParseAccess() = %v, want %v", parsed, userID)
	}
}

func TestParseRejectsExpiredToken(t *testing.T) {
	issuer := token.New(testSecret, -accessTokenTTL)

	raw, _, err := issuer.IssueAccess(uuid.New())
	if err != nil {
		t.Fatalf("IssueAccess() error = %v", err)
	}

	if _, err := issuer.ParseAccess(raw); !errors.Is(err, rules.ErrTokenExpired) {
		t.Fatalf("ParseAccess() error = %v, want ErrTokenExpired", err)
	}
}

func TestParseRejectsForeignSecret(t *testing.T) {
	issuer := token.New(testSecret, accessTokenTTL)
	other := token.New(anotherSecret, accessTokenTTL)

	raw, _, err := issuer.IssueAccess(uuid.New())
	if err != nil {
		t.Fatalf("IssueAccess() error = %v", err)
	}

	if _, err := other.ParseAccess(raw); !errors.Is(err, rules.ErrUnauthenticated) {
		t.Fatalf("ParseAccess() error = %v, want ErrUnauthenticated", err)
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	issuer := token.New(testSecret, accessTokenTTL)

	for _, raw := range []string{"", "not-a-jwt", "a.b.c"} {
		if _, err := issuer.ParseAccess(raw); !errors.Is(err, rules.ErrUnauthenticated) {
			t.Fatalf("ParseAccess(%q) error = %v, want ErrUnauthenticated", raw, err)
		}
	}
}
