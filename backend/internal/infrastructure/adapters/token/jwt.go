// Package token issues and parses JWT access tokens.
package token

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
)

const (
	issuerName       = "rag-movie-picker"
	accessTokenType  = "access"
	signingAlgorithm = "HS256"
)

type claims struct {
	Type string `json:"typ"`
	jwt.RegisteredClaims
}

// Issuer signs access tokens with HS256.
type Issuer struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// New builds the issuer. The secret is validated by the configuration loader.
func New(secret string, ttl time.Duration) *Issuer {
	return &Issuer{secret: []byte(secret), ttl: ttl, now: time.Now}
}

// IssueAccess returns a signed token and its lifetime in seconds.
func (i *Issuer) IssueAccess(userID uuid.UUID) (string, int, error) {
	now := i.now()
	payload := claims{
		Type: accessTokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Issuer:    issuerName,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(i.ttl)),
		},
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, payload).SignedString(i.secret)
	if err != nil {
		return "", 0, fmt.Errorf("sign access token: %w", err)
	}
	return signed, int(i.ttl.Seconds()), nil
}

// ParseAccess verifies the signature, the issuer, the token type and the expiry.
func (i *Issuer) ParseAccess(raw string) (uuid.UUID, error) {
	parsed, err := jwt.ParseWithClaims(raw, &claims{}, func(*jwt.Token) (any, error) {
		return i.secret, nil
	},
		jwt.WithValidMethods([]string{signingAlgorithm}),
		jwt.WithIssuer(issuerName),
		jwt.WithTimeFunc(i.now),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return uuid.Nil, rules.ErrTokenExpired
		}
		return uuid.Nil, rules.ErrUnauthenticated
	}

	payload, ok := parsed.Claims.(*claims)
	if !ok || !parsed.Valid || payload.Type != accessTokenType {
		return uuid.Nil, rules.ErrUnauthenticated
	}

	userID, err := uuid.Parse(payload.Subject)
	if err != nil {
		return uuid.Nil, rules.ErrUnauthenticated
	}
	return userID, nil
}
