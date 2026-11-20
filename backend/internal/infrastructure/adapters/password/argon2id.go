// Package password hashes and verifies passwords with Argon2id.
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	saltLength = 16
	keyLength  = 32
	version    = 19
)

// Hasher implements the password hashing contract with the Argon2id PHC string
// format, so that the cost parameters travel with every hash.
type Hasher struct {
	iterations  uint32
	memoryKiB   uint32
	parallelism uint8
}

// New builds a hasher. The memory is passed in mebibytes for configuration
// readability and converted to the kibibytes required by Argon2.
func New(iterations, memoryMiB uint32, parallelism uint8) *Hasher {
	return &Hasher{
		iterations:  iterations,
		memoryKiB:   memoryMiB * 1024,
		parallelism: parallelism,
	}
}

// Hash returns a salted PHC string.
func (h *Hasher) Hash(password string) (string, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("read salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, h.iterations, h.memoryKiB, h.parallelism, keyLength)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		version,
		h.memoryKiB,
		h.iterations,
		h.parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// Verify compares the password with the stored hash in constant time.
func (h *Hasher) Verify(password, encoded string) (bool, error) {
	params, salt, key, err := decode(encoded)
	if err != nil {
		return false, err
	}

	// The key length comes from a stored PHC string, which is bounded by the
	// database column, so the conversion cannot overflow.
	//nolint:gosec // G115: see above.
	computed := argon2.IDKey([]byte(password), salt, params.iterations, params.memoryKiB, params.parallelism, uint32(len(key)))
	return subtle.ConstantTimeCompare(key, computed) == 1, nil
}

type parameters struct {
	iterations  uint32
	memoryKiB   uint32
	parallelism uint8
}

func decode(encoded string) (parameters, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return parameters{}, nil, nil, errors.New("invalid argon2id hash")
	}

	var parsedVersion int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &parsedVersion); err != nil || parsedVersion != version {
		return parameters{}, nil, nil, errors.New("unsupported argon2id version")
	}

	var (
		memoryKiB   uint32
		iterations  uint32
		parallelism uint8
	)
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memoryKiB, &iterations, &parallelism); err != nil {
		return parameters{}, nil, nil, fmt.Errorf("parse argon2id parameters: %w", err)
	}
	if memoryKiB == 0 || iterations == 0 || parallelism == 0 {
		return parameters{}, nil, nil, errors.New("invalid argon2id parameters")
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return parameters{}, nil, nil, errors.New("invalid argon2id salt")
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return parameters{}, nil, nil, errors.New("invalid argon2id key")
	}

	return parameters{iterations: iterations, memoryKiB: memoryKiB, parallelism: parallelism}, salt, key, nil
}
