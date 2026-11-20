package password_test

import (
	"strings"
	"testing"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/adapters/password"
)

// newHasher keeps the tests fast: the cost parameters are irrelevant for
// correctness, only the format and verification matter.
func newHasher() *password.Hasher {
	return password.New(1, 8, 1)
}

func TestHashAndVerify(t *testing.T) {
	hasher := newHasher()

	hash, err := hasher.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatalf("Hash() = %q, want an argon2id PHC string", hash)
	}

	valid, err := hasher.Verify("correct horse battery staple", hash)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !valid {
		t.Fatal("Verify() must accept the original password")
	}

	valid, err = hasher.Verify("wrong password", hash)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if valid {
		t.Fatal("Verify() must reject another password")
	}
}

func TestHashIsSalted(t *testing.T) {
	hasher := newHasher()

	first, err := hasher.Hash("same-password")
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	second, err := hasher.Hash("same-password")
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	if first == second {
		t.Fatal("two hashes of one password must differ")
	}

	valid, err := hasher.Verify("same-password", second)
	if err != nil || !valid {
		t.Fatalf("Verify() = %t, %v", valid, err)
	}
}

func TestVerifyRejectsMalformedHash(t *testing.T) {
	hasher := newHasher()

	cases := []string{
		"",
		"plain-password",
		"$argon2i$v=19$m=8192,t=1,p=1$c2FsdA$a2V5",
		"$argon2id$v=16$m=8192,t=1,p=1$c2FsdA$a2V5",
		"$argon2id$v=19$m=8192$c2FsdA$a2V5",
		"$argon2id$v=19$m=0,t=1,p=1$c2FsdA$a2V5",
		"$argon2id$v=19$m=8192,t=1,p=1$!!!$a2V5",
		"$argon2id$v=19$m=8192,t=1,p=1$c2FsdA$!!!",
	}
	for _, hash := range cases {
		if _, err := hasher.Verify("password", hash); err == nil {
			t.Fatalf("Verify(%q) must fail", hash)
		}
	}
}
