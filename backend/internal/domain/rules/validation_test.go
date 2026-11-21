package rules_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
)

func TestNormalizeEmail(t *testing.T) {
	if got := rules.NormalizeEmail("  User@Example.COM "); got != "user@example.com" {
		t.Fatalf("NormalizeEmail() = %q", got)
	}
}

func TestValidateEmailAcceptsCommonAddresses(t *testing.T) {
	for _, email := range []string{"user@example.com", "first.last+tag@sub.example.co"} {
		if err := rules.ValidateEmail(email); err != nil {
			t.Fatalf("ValidateEmail(%q) error = %v", email, err)
		}
	}
}

func TestValidateEmailRejectsMalformedAddresses(t *testing.T) {
	invalid := []string{
		"",
		"user",
		"@example.com",
		"user@",
		"user@example",
		"user name@example.com",
		"user@.example.com",
		"user@example.com.",
		strings.Repeat("a", 250) + "@example.com",
	}
	for _, email := range invalid {
		err := rules.ValidateEmail(email)
		if err == nil {
			t.Fatalf("ValidateEmail(%q) must fail", email)
		}
		var validationErr *rules.ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("ValidateEmail(%q) error = %T, want *rules.ValidationError", email, err)
		}
		if validationErr.Message == "" {
			t.Fatalf("ValidateEmail(%q) message is empty", email)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	for _, password := range []string{"password", "длинный пароль", strings.Repeat("a", 128)} {
		if err := rules.ValidatePassword(password); err != nil {
			t.Fatalf("ValidatePassword(%q) error = %v", password, err)
		}
	}

	for _, password := range []string{"", "short", strings.Repeat("a", 129)} {
		err := rules.ValidatePassword(password)
		if err == nil {
			t.Fatalf("ValidatePassword(%q) must fail", password)
		}
		var validationErr *rules.ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("ValidatePassword error = %T, want *rules.ValidationError", err)
		}
	}
}
