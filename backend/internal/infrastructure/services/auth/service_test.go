package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/services/auth"
)

const (
	testEmail           = "user@example.com"
	testPassword        = "password123"
	testRefreshLifetime = 30 * 24 * time.Hour
	dummyPassword       = "dummy-password-for-timing"
)

var (
	testUserID    = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	testCreatedAt = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
)

type fakeUsers struct {
	createErr error
	created   models.User
	byEmail   models.User
	emailErr  error
	byID      models.User
	idErr     error
}

func (f *fakeUsers) Create(_ context.Context, user models.User) (models.User, error) {
	if f.createErr != nil {
		return models.User{}, f.createErr
	}
	user.ID = testUserID
	user.CreatedAt = testCreatedAt
	f.created = user
	return user, nil
}

func (f *fakeUsers) FindByEmail(context.Context, string) (models.User, error) {
	return f.byEmail, f.emailErr
}

func (f *fakeUsers) FindByID(context.Context, uuid.UUID) (models.User, error) {
	return f.byID, f.idErr
}

type fakeTokens struct {
	created      []models.RefreshToken
	createErr    error
	stored       models.RefreshToken
	hashErr      error
	revoked      []uuid.UUID
	revokeResult bool
	revokeErr    error
	families     []uuid.UUID
	familyErr    error
}

func (f *fakeTokens) Create(_ context.Context, token models.RefreshToken) (models.RefreshToken, error) {
	if f.createErr != nil {
		return models.RefreshToken{}, f.createErr
	}
	token.ID = uuid.New()
	token.CreatedAt = testCreatedAt
	f.created = append(f.created, token)
	return token, nil
}

func (f *fakeTokens) FindByHash(context.Context, []byte) (models.RefreshToken, error) {
	return f.stored, f.hashErr
}

func (f *fakeTokens) Revoke(_ context.Context, id uuid.UUID) (bool, error) {
	f.revoked = append(f.revoked, id)
	return f.revokeResult, f.revokeErr
}

func (f *fakeTokens) RevokeFamily(_ context.Context, familyID uuid.UUID) (int64, error) {
	f.families = append(f.families, familyID)
	if f.familyErr != nil {
		return 0, f.familyErr
	}
	return 1, nil
}

func (f *fakeTokens) DeleteExpired(context.Context, time.Time) (int64, error) { return 0, nil }

type fakeHasher struct {
	hashResult   string
	hashErr      error
	hashInputs   []string
	verifyResult bool
	verifyErr    error
	verifyInputs []string
	verifyHashes []string
}

func (f *fakeHasher) Hash(password string) (string, error) {
	f.hashInputs = append(f.hashInputs, password)
	if f.hashErr != nil {
		return "", f.hashErr
	}
	if f.hashResult == "" {
		return "$dummy-hash", nil
	}
	return f.hashResult, nil
}

func (f *fakeHasher) Verify(password, hash string) (bool, error) {
	f.verifyInputs = append(f.verifyInputs, password)
	f.verifyHashes = append(f.verifyHashes, hash)
	return f.verifyResult, f.verifyErr
}

type fakeIssuer struct {
	err error
}

func (fakeIssuer) IssueAccessToken(uuid.UUID) (string, int, error) { return "access-token", 900, nil }

func (f fakeIssuer) ParseAccessToken(string) (uuid.UUID, error) { return uuid.Nil, f.err }

func newService(t *testing.T, users *fakeUsers, tokens *fakeTokens, hasher *fakeHasher) *auth.AuthenticationService {
	t.Helper()

	service, err := auth.NewAuthenticationService(users, tokens, hasher, fakeIssuer{}, testRefreshLifetime)
	if err != nil {
		t.Fatalf("auth.NewAuthenticationService() error = %v", err)
	}
	return service
}

func activeToken(family uuid.UUID) models.RefreshToken {
	return models.RefreshToken{
		ID:        uuid.New(),
		UserID:    testUserID,
		TokenHash: []byte("hash"),
		FamilyID:  family,
		ExpiresAt: time.Now().Add(time.Hour),
	}
}

func TestRegisterNormalizesEmailAndStartsSession(t *testing.T) {
	users := &fakeUsers{}
	tokens := &fakeTokens{}
	hasher := &fakeHasher{}
	service := newService(t, users, tokens, hasher)

	session, err := service.Register(context.Background(), "  User@Example.COM ", testPassword)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if users.created.Email != testEmail {
		t.Fatalf("created email = %q, want %q", users.created.Email, testEmail)
	}
	if session.User.ID != testUserID {
		t.Fatalf("session user id = %v", session.User.ID)
	}
	if session.AccessToken != "access-token" || session.AccessTokenExpiresIn != 900 {
		t.Fatalf("access token = %q, %d", session.AccessToken, session.AccessTokenExpiresIn)
	}
	if session.RefreshToken == "" || session.RefreshTokenExpiresAt.IsZero() {
		t.Fatal("refresh token and expiry must be set")
	}
	if len(tokens.created) != 1 {
		t.Fatalf("stored tokens = %d, want 1", len(tokens.created))
	}
	if tokens.created[0].FamilyID == uuid.Nil {
		t.Fatal("refresh token family must be set")
	}
	if len(tokens.created[0].TokenHash) != sha256Length {
		t.Fatalf("token hash length = %d, want %d", len(tokens.created[0].TokenHash), sha256Length)
	}
	if len(hasher.hashInputs) != 2 || hasher.hashInputs[1] != testPassword {
		t.Fatalf("hashed passwords = %v", hasher.hashInputs)
	}
}

const sha256Length = 32

func TestRegisterValidatesInput(t *testing.T) {
	cases := map[string]struct{ email, password string }{
		"bad email":      {"not-an-email", testPassword},
		"short password": {testEmail, "short"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			users := &fakeUsers{}
			service := newService(t, users, &fakeTokens{}, &fakeHasher{})

			_, err := service.Register(context.Background(), testCase.email, testCase.password)
			var validationErr *rules.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("Register() error = %v, want *rules.ValidationError", err)
			}
			if users.created.Email != "" {
				t.Fatal("invalid input must not reach the repository")
			}
		})
	}
}

func TestRegisterReportsTakenEmail(t *testing.T) {
	users := &fakeUsers{createErr: rules.ErrEmailTaken}
	service := newService(t, users, &fakeTokens{}, &fakeHasher{})

	_, err := service.Register(context.Background(), testEmail, testPassword)
	if !errors.Is(err, rules.ErrEmailTaken) {
		t.Fatalf("Register() error = %v, want ErrEmailTaken", err)
	}
}

func TestLoginVerifiesDummyHashForUnknownEmail(t *testing.T) {
	users := &fakeUsers{emailErr: rules.ErrNotFound}
	hasher := &fakeHasher{verifyResult: true}
	service := newService(t, users, &fakeTokens{}, hasher)

	_, err := service.Login(context.Background(), testEmail, testPassword)
	if !errors.Is(err, rules.ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want ErrInvalidCredentials", err)
	}
	if len(hasher.verifyHashes) != 1 || hasher.verifyHashes[0] != "$dummy-hash" {
		t.Fatalf("verified hashes = %v, want the dummy hash", hasher.verifyHashes)
	}
	if len(hasher.verifyInputs) != 1 || hasher.verifyInputs[0] != testPassword {
		t.Fatalf("verified passwords = %v", hasher.verifyInputs)
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	users := &fakeUsers{byEmail: models.User{ID: testUserID, Email: testEmail, PasswordHash: "$hash"}}
	hasher := &fakeHasher{verifyResult: false}
	service := newService(t, users, &fakeTokens{}, hasher)

	_, err := service.Login(context.Background(), testEmail, testPassword)
	if !errors.Is(err, rules.ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want ErrInvalidCredentials", err)
	}
}

func TestLoginStartsSession(t *testing.T) {
	users := &fakeUsers{byEmail: models.User{ID: testUserID, Email: testEmail, PasswordHash: "$hash"}}
	tokens := &fakeTokens{}
	service := newService(t, users, tokens, &fakeHasher{verifyResult: true})

	session, err := service.Login(context.Background(), "USER@example.com", testPassword)
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if session.AccessToken == "" || session.RefreshToken == "" {
		t.Fatal("session tokens must be set")
	}
	if len(tokens.created) != 1 {
		t.Fatalf("stored tokens = %d, want 1", len(tokens.created))
	}
}

func TestLoginRequiresCredentials(t *testing.T) {
	users := &fakeUsers{}
	service := newService(t, users, &fakeTokens{}, &fakeHasher{})

	_, err := service.Login(context.Background(), "", "")
	var validationErr *rules.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("Login() error = %v, want *rules.ValidationError", err)
	}
}

func TestRefreshRotatesWithinFamily(t *testing.T) {
	family := uuid.New()
	stored := activeToken(family)
	tokens := &fakeTokens{stored: stored, revokeResult: true}
	users := &fakeUsers{byID: models.User{ID: testUserID, Email: testEmail}}
	service := newService(t, users, tokens, &fakeHasher{})

	session, err := service.Refresh(context.Background(), "old-refresh-token")
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if len(tokens.revoked) != 1 || tokens.revoked[0] != stored.ID {
		t.Fatalf("revoked tokens = %v, want %v", tokens.revoked, stored.ID)
	}
	if len(tokens.created) != 1 || tokens.created[0].FamilyID != family {
		t.Fatalf("new token = %+v, want the same family %v", tokens.created, family)
	}
	if session.RefreshToken == "old-refresh-token" {
		t.Fatal("refresh token must be rotated")
	}
	if session.AccessToken == "" {
		t.Fatal("access token must be issued")
	}
}

func TestRefreshRejectsExpiredToken(t *testing.T) {
	expired := activeToken(uuid.New())
	expired.ExpiresAt = time.Now().Add(-time.Minute)
	tokens := &fakeTokens{stored: expired, revokeResult: true}
	service := newService(t, &fakeUsers{}, tokens, &fakeHasher{})

	_, err := service.Refresh(context.Background(), "expired-token")
	if !errors.Is(err, rules.ErrUnauthenticated) {
		t.Fatalf("Refresh() error = %v, want ErrUnauthenticated", err)
	}
	if len(tokens.revoked) != 0 {
		t.Fatal("expired token must not be rotated")
	}
}

func TestRefreshRevokesFamilyWhenTokenIsReused(t *testing.T) {
	family := uuid.New()
	revokedAt := time.Now().Add(-time.Minute)
	stored := activeToken(family)
	stored.RevokedAt = &revokedAt
	tokens := &fakeTokens{stored: stored}
	service := newService(t, &fakeUsers{}, tokens, &fakeHasher{})

	_, err := service.Refresh(context.Background(), "reused-token")
	if !errors.Is(err, rules.ErrUnauthenticated) {
		t.Fatalf("Refresh() error = %v, want ErrUnauthenticated", err)
	}
	if len(tokens.families) != 1 || tokens.families[0] != family {
		t.Fatalf("revoked families = %v, want %v", tokens.families, family)
	}
	if len(tokens.created) != 0 {
		t.Fatal("reused token must not produce a new session")
	}
}

func TestRefreshRejectsUnknownToken(t *testing.T) {
	tokens := &fakeTokens{hashErr: rules.ErrNotFound}
	service := newService(t, &fakeUsers{}, tokens, &fakeHasher{})

	_, err := service.Refresh(context.Background(), "unknown")
	if !errors.Is(err, rules.ErrUnauthenticated) {
		t.Fatalf("Refresh() error = %v, want ErrUnauthenticated", err)
	}
}

func TestRefreshRevokesFamilyOnConcurrentRotation(t *testing.T) {
	family := uuid.New()
	tokens := &fakeTokens{stored: activeToken(family), revokeResult: false}
	service := newService(t, &fakeUsers{}, tokens, &fakeHasher{})

	_, err := service.Refresh(context.Background(), "raced-token")
	if !errors.Is(err, rules.ErrUnauthenticated) {
		t.Fatalf("Refresh() error = %v, want ErrUnauthenticated", err)
	}
	if len(tokens.families) != 1 || tokens.families[0] != family {
		t.Fatalf("revoked families = %v, want %v", tokens.families, family)
	}
}

func TestRefreshRejectsDeletedUser(t *testing.T) {
	tokens := &fakeTokens{stored: activeToken(uuid.New()), revokeResult: true}
	users := &fakeUsers{idErr: rules.ErrNotFound}
	service := newService(t, users, tokens, &fakeHasher{})

	_, err := service.Refresh(context.Background(), "token")
	if !errors.Is(err, rules.ErrUnauthenticated) {
		t.Fatalf("Refresh() error = %v, want ErrUnauthenticated", err)
	}
}

func TestLogoutIsIdempotent(t *testing.T) {
	t.Run("empty token", func(t *testing.T) {
		tokens := &fakeTokens{}
		service := newService(t, &fakeUsers{}, tokens, &fakeHasher{})

		if err := service.Logout(context.Background(), ""); err != nil {
			t.Fatalf("Logout() error = %v", err)
		}
		if len(tokens.revoked) != 0 {
			t.Fatal("empty token must not be revoked")
		}
	})

	t.Run("unknown token", func(t *testing.T) {
		tokens := &fakeTokens{hashErr: rules.ErrNotFound}
		service := newService(t, &fakeUsers{}, tokens, &fakeHasher{})

		if err := service.Logout(context.Background(), "unknown"); err != nil {
			t.Fatalf("Logout() error = %v", err)
		}
	})

	t.Run("active token", func(t *testing.T) {
		stored := activeToken(uuid.New())
		tokens := &fakeTokens{stored: stored}
		service := newService(t, &fakeUsers{}, tokens, &fakeHasher{})

		if err := service.Logout(context.Background(), "token"); err != nil {
			t.Fatalf("Logout() error = %v", err)
		}
		if len(tokens.revoked) != 1 || tokens.revoked[0] != stored.ID {
			t.Fatalf("revoked tokens = %v, want %v", tokens.revoked, stored.ID)
		}
	})
}

func TestMe(t *testing.T) {
	user := models.User{ID: testUserID, Email: testEmail}

	service := newService(t, &fakeUsers{byID: user}, &fakeTokens{}, &fakeHasher{})
	found, err := service.CurrentUser(context.Background(), testUserID)
	if err != nil || found.Email != testEmail {
		t.Fatalf("CurrentUser() = %+v, %v", found, err)
	}

	missing := newService(t, &fakeUsers{idErr: rules.ErrNotFound}, &fakeTokens{}, &fakeHasher{})
	if _, err := missing.CurrentUser(context.Background(), testUserID); !errors.Is(err, rules.ErrNotFound) {
		t.Fatalf("CurrentUser() error = %v, want ErrNotFound", err)
	}
}

func TestNewFailsWhenDummyHashFails(t *testing.T) {
	hasher := &fakeHasher{hashErr: errors.New("hashing unavailable")}

	if _, err := auth.NewAuthenticationService(&fakeUsers{}, &fakeTokens{}, hasher, fakeIssuer{}, testRefreshLifetime); err == nil {
		t.Fatal("auth.NewAuthenticationService() must fail when the dummy hash cannot be built")
	}
}
