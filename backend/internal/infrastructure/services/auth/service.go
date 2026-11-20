// Package auth implements registration, login and refresh token rotation.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/ports"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
)

const refreshTokenBytes = 32

// Service implements ports.AuthService.
type Service struct {
	users      ports.UserRepository
	tokens     ports.RefreshTokenRepository
	hasher     ports.PasswordHasher
	issuer     ports.TokenIssuer
	refreshTTL time.Duration
	now        func() time.Time
	dummyHash  string
}

// New builds the service. The dummy hash keeps the login timing similar for
// known and unknown emails: it is verified when the account does not exist.
func New(
	users ports.UserRepository,
	tokens ports.RefreshTokenRepository,
	hasher ports.PasswordHasher,
	issuer ports.TokenIssuer,
	refreshTTL time.Duration,
) (*Service, error) {
	dummyHash, err := hasher.Hash("dummy-password-for-timing")
	if err != nil {
		return nil, fmt.Errorf("hash dummy password: %w", err)
	}
	return &Service{
		users:      users,
		tokens:     tokens,
		hasher:     hasher,
		issuer:     issuer,
		refreshTTL: refreshTTL,
		now:        time.Now,
		dummyHash:  dummyHash,
	}, nil
}

// Register creates an account and starts a session.
func (s *Service) Register(ctx context.Context, email, password string) (models.Session, error) {
	normalized := rules.NormalizeEmail(email)
	if err := rules.ValidateEmail(normalized); err != nil {
		return models.Session{}, err
	}
	if err := rules.ValidatePassword(password); err != nil {
		return models.Session{}, err
	}

	hash, err := s.hasher.Hash(password)
	if err != nil {
		return models.Session{}, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.users.Create(ctx, models.User{Email: normalized, PasswordHash: hash})
	if err != nil {
		return models.Session{}, err
	}

	return s.newSession(ctx, user, uuid.New())
}

// Login verifies the credentials and starts a session.
func (s *Service) Login(ctx context.Context, email, password string) (models.Session, error) {
	normalized := rules.NormalizeEmail(email)
	if normalized == "" || password == "" {
		return models.Session{}, &rules.ValidationError{Message: "Укажите адрес электронной почты и пароль."}
	}

	user, err := s.users.ByEmail(ctx, normalized)
	if err != nil {
		if errors.Is(err, rules.ErrNotFound) {
			if _, verifyErr := s.hasher.Verify(password, s.dummyHash); verifyErr != nil {
				return models.Session{}, fmt.Errorf("verify dummy password: %w", verifyErr)
			}
			return models.Session{}, rules.ErrInvalidCredentials
		}
		return models.Session{}, err
	}

	valid, err := s.hasher.Verify(password, user.PasswordHash)
	if err != nil {
		return models.Session{}, fmt.Errorf("verify password: %w", err)
	}
	if !valid {
		return models.Session{}, rules.ErrInvalidCredentials
	}

	return s.newSession(ctx, user, uuid.New())
}

// Refresh rotates the refresh token and returns a new session.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (models.Session, error) {
	stored, err := s.tokens.ByHash(ctx, hashToken(refreshToken))
	if err != nil {
		if errors.Is(err, rules.ErrNotFound) {
			return models.Session{}, rules.ErrUnauthenticated
		}
		return models.Session{}, err
	}

	now := s.now()
	if stored.Revoked() {
		// A rotated token was presented again: treat it as stolen and revoke
		// every token of the family.
		if _, err := s.tokens.RevokeFamily(ctx, stored.FamilyID); err != nil {
			return models.Session{}, err
		}
		return models.Session{}, rules.ErrUnauthenticated
	}
	if !stored.ExpiresAt.After(now) {
		return models.Session{}, rules.ErrUnauthenticated
	}

	revoked, err := s.tokens.Revoke(ctx, stored.ID)
	if err != nil {
		return models.Session{}, err
	}
	if !revoked {
		// The same token was rotated by a concurrent request.
		if _, err := s.tokens.RevokeFamily(ctx, stored.FamilyID); err != nil {
			return models.Session{}, err
		}
		return models.Session{}, rules.ErrUnauthenticated
	}

	user, err := s.users.ByID(ctx, stored.UserID)
	if err != nil {
		if errors.Is(err, rules.ErrNotFound) {
			return models.Session{}, rules.ErrUnauthenticated
		}
		return models.Session{}, err
	}

	return s.newSession(ctx, user, stored.FamilyID)
}

// Logout revokes the refresh token. It is idempotent: an unknown or already
// revoked token is not an error.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}

	stored, err := s.tokens.ByHash(ctx, hashToken(refreshToken))
	if err != nil {
		if errors.Is(err, rules.ErrNotFound) {
			return nil
		}
		return err
	}
	if _, err := s.tokens.Revoke(ctx, stored.ID); err != nil {
		return err
	}
	return nil
}

// Me returns the account of the authenticated user.
func (s *Service) Me(ctx context.Context, userID uuid.UUID) (models.User, error) {
	return s.users.ByID(ctx, userID)
}

func (s *Service) newSession(ctx context.Context, user models.User, familyID uuid.UUID) (models.Session, error) {
	refreshToken, err := generateRefreshToken()
	if err != nil {
		return models.Session{}, err
	}

	expiresAt := s.now().Add(s.refreshTTL)
	if _, err := s.tokens.Create(ctx, models.RefreshToken{
		UserID:    user.ID,
		TokenHash: hashToken(refreshToken),
		FamilyID:  familyID,
		ExpiresAt: expiresAt,
	}); err != nil {
		return models.Session{}, err
	}

	accessToken, expiresIn, err := s.issuer.IssueAccess(user.ID)
	if err != nil {
		return models.Session{}, err
	}

	return models.Session{
		User:             user,
		AccessToken:      accessToken,
		AccessExpiresIn:  expiresIn,
		RefreshToken:     refreshToken,
		RefreshExpiresAt: expiresAt,
	}, nil
}

func generateRefreshToken() (string, error) {
	buffer := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("read refresh token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
