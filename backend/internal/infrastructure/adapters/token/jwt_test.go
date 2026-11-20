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
	issuer := token.NewAccessTokenIssuer(testSecret, accessTokenTTL)
	userID := uuid.New()

	raw, expiresIn, err := issuer.IssueAccessToken(userID)
	if err != nil {
		t.Fatalf("IssueAccessToken() error = %v", err)
	}
	if expiresIn != 60 {
		t.Fatalf("expiresIn = %d, want 60", expiresIn)
	}

	parsed, err := issuer.ParseAccessToken(raw)
	if err != nil {
		t.Fatalf("ParseAccessToken() error = %v", err)
	}
	if parsed != userID {
		t.Fatalf("ParseAccessToken() = %v, want %v", parsed, userID)
	}
}

func TestParseRejectsExpiredToken(t *testing.T) {
	issuer := token.NewAccessTokenIssuer(testSecret, -accessTokenTTL)

	raw, _, err := issuer.IssueAccessToken(uuid.New())
	if err != nil {
		t.Fatalf("IssueAccessToken() error = %v", err)
	}

	if _, err := issuer.ParseAccessToken(raw); !errors.Is(err, rules.ErrTokenExpired) {
		t.Fatalf("ParseAccessToken() error = %v, want ErrTokenExpired", err)
	}
}

func TestParseRejectsForeignSecret(t *testing.T) {
	issuer := token.NewAccessTokenIssuer(testSecret, accessTokenTTL)
	other := token.NewAccessTokenIssuer(anotherSecret, accessTokenTTL)

	raw, _, err := issuer.IssueAccessToken(uuid.New())
	if err != nil {
		t.Fatalf("IssueAccessToken() error = %v", err)
	}

	if _, err := other.ParseAccessToken(raw); !errors.Is(err, rules.ErrUnauthenticated) {
		t.Fatalf("ParseAccessToken() error = %v, want ErrUnauthenticated", err)
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	issuer := token.NewAccessTokenIssuer(testSecret, accessTokenTTL)

	for _, raw := range []string{"", "not-a-jwt", "a.b.c"} {
		if _, err := issuer.ParseAccessToken(raw); !errors.Is(err, rules.ErrUnauthenticated) {
			t.Fatalf("ParseAccessToken(%q) error = %v, want ErrUnauthenticated", raw, err)
		}
	}
}
