package handlers_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/handlers"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server/middleware"
	"github.com/ivanSaichkin/rag-movie-picker/backend/pkg/testutils"
)

var (
	testUserID    = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	testCreatedAt = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
)

type fakeAuthService struct {
	session models.Session
	authErr error
	user    models.User
	meErr   error

	registerEmail    string
	registerPassword string
	loginEmail       string
	loginPassword    string
	refreshInput     string
	logoutInput      string
	registerCalls    int
	loginCalls       int
	refreshCalls     int
	logoutCalls      int
}

func (f *fakeAuthService) Register(_ context.Context, email, password string) (models.Session, error) {
	f.registerCalls++
	f.registerEmail = email
	f.registerPassword = password
	return f.session, f.authErr
}

func (f *fakeAuthService) Login(_ context.Context, email, password string) (models.Session, error) {
	f.loginCalls++
	f.loginEmail = email
	f.loginPassword = password
	return f.session, f.authErr
}

func (f *fakeAuthService) Refresh(_ context.Context, refreshToken string) (models.Session, error) {
	f.refreshCalls++
	f.refreshInput = refreshToken
	return f.session, f.authErr
}

func (f *fakeAuthService) Logout(_ context.Context, refreshToken string) error {
	f.logoutCalls++
	f.logoutInput = refreshToken
	return f.authErr
}

func (f *fakeAuthService) CurrentUser(context.Context, uuid.UUID) (models.User, error) {
	return f.user, f.meErr
}

type fakeIssuer struct {
	userID uuid.UUID
	err    error
}

func (f fakeIssuer) IssueAccessToken(uuid.UUID) (string, int, error) { return "", 0, nil }

func (f fakeIssuer) ParseAccessToken(string) (uuid.UUID, error) { return f.userID, f.err }

func testSession() models.Session {
	return models.Session{
		User:                  models.User{ID: testUserID, Email: "user@example.com", CreatedAt: testCreatedAt},
		AccessToken:           "access-token",
		AccessTokenExpiresIn:  900,
		RefreshToken:          "refresh-token-1",
		RefreshTokenExpiresAt: time.Now().Add(24 * time.Hour),
	}
}

func newAuthEngine(service *fakeAuthService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.Logging(slog.New(slog.NewJSONHandler(io.Discard, nil))))

	auth := handlers.NewAuthHandler(service, false)
	engine.POST("/register", auth.Register)
	engine.POST("/login", auth.Login)
	engine.POST("/refresh", auth.Refresh)
	engine.POST("/logout", auth.Logout)
	return engine
}

func newMeEngine(service *fakeAuthService, issuer fakeIssuer) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.Logging(slog.New(slog.NewJSONHandler(io.Discard, nil))))

	auth := handlers.NewAuthHandler(service, false)
	engine.GET("/me", middleware.Authenticate(issuer), auth.CurrentUser)
	return engine
}

func TestRegisterCreatesSessionAndCookie(t *testing.T) {
	service := &fakeAuthService{session: testSession()}
	engine := newAuthEngine(service)

	recorder := testutils.Perform(t, engine, http.MethodPost, "/register",
		strings.NewReader(`{"email":"user@example.com","password":"password123"}`), jsonHeader())

	testutils.RequireStatus(t, recorder, http.StatusCreated)

	body := testutils.DecodeJSON[dto.AuthResponse](t, recorder)
	if body.AccessToken != "access-token" || body.AccessTokenExpiresIn != 900 {
		t.Fatalf("auth response = %+v", body)
	}
	if body.User.Email != "user@example.com" {
		t.Fatalf("user = %+v", body.User)
	}
	if strings.Contains(recorder.Body.String(), "refresh-token-1") {
		t.Fatal("the refresh token must not appear in the response body")
	}

	cookie := refreshCookie(t, recorder)
	if cookie.Value != "refresh-token-1" {
		t.Fatalf("cookie value = %q", cookie.Value)
	}
	if !cookie.HttpOnly {
		t.Fatal("the refresh cookie must be HttpOnly")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("SameSite = %v, want Strict", cookie.SameSite)
	}
	if cookie.Path != "/api/v1/auth" {
		t.Fatalf("cookie path = %q", cookie.Path)
	}
	if cookie.Secure {
		t.Fatal("Secure must follow the configuration, false in tests")
	}

	if service.registerEmail != "user@example.com" || service.registerPassword != "password123" {
		t.Fatalf("service received %q, %q", service.registerEmail, service.registerPassword)
	}
}

func TestRegisterMapsValidationError(t *testing.T) {
	service := &fakeAuthService{authErr: &rules.ValidationError{Message: "Пароль слишком короткий."}}
	engine := newAuthEngine(service)

	recorder := testutils.Perform(t, engine, http.MethodPost, "/register",
		strings.NewReader(`{"email":"user@example.com","password":"x"}`), jsonHeader())

	testutils.RequireStatus(t, recorder, http.StatusUnprocessableEntity)

	body := testutils.DecodeJSON[dto.ErrorResponse](t, recorder)
	if body.Error.Code != dto.CodeValidationError {
		t.Fatalf("code = %q", body.Error.Code)
	}
	if body.Error.Message != "Пароль слишком короткий." {
		t.Fatalf("message = %q", body.Error.Message)
	}
}

func TestRegisterMapsEmailTaken(t *testing.T) {
	service := &fakeAuthService{authErr: rules.ErrEmailTaken}
	engine := newAuthEngine(service)

	recorder := testutils.Perform(t, engine, http.MethodPost, "/register",
		strings.NewReader(`{"email":"user@example.com","password":"password123"}`), jsonHeader())

	testutils.RequireStatus(t, recorder, http.StatusConflict)

	body := testutils.DecodeJSON[dto.ErrorResponse](t, recorder)
	if body.Error.Code != dto.CodeEmailTaken {
		t.Fatalf("code = %q, want %q", body.Error.Code, dto.CodeEmailTaken)
	}
}

func TestRegisterRejectsMalformedJSON(t *testing.T) {
	service := &fakeAuthService{session: testSession()}
	engine := newAuthEngine(service)

	recorder := testutils.Perform(t, engine, http.MethodPost, "/register", strings.NewReader(`{`), jsonHeader())

	testutils.RequireStatus(t, recorder, http.StatusUnprocessableEntity)
	if service.registerCalls != 0 {
		t.Fatal("malformed JSON must not reach the service")
	}
}

func TestLoginMapsInvalidCredentials(t *testing.T) {
	service := &fakeAuthService{authErr: rules.ErrInvalidCredentials}
	engine := newAuthEngine(service)

	recorder := testutils.Perform(t, engine, http.MethodPost, "/login",
		strings.NewReader(`{"email":"user@example.com","password":"wrong"}`), jsonHeader())

	testutils.RequireStatus(t, recorder, http.StatusUnauthorized)

	body := testutils.DecodeJSON[dto.ErrorResponse](t, recorder)
	if body.Error.Code != dto.CodeInvalidCredentials {
		t.Fatalf("code = %q", body.Error.Code)
	}
	if len(recorder.Result().Cookies()) != 0 {
		t.Fatal("failed login must not set the refresh cookie")
	}
}

func TestLoginReturnsAccessToken(t *testing.T) {
	service := &fakeAuthService{session: testSession()}
	engine := newAuthEngine(service)

	recorder := testutils.Perform(t, engine, http.MethodPost, "/login",
		strings.NewReader(`{"email":"user@example.com","password":"password123"}`), jsonHeader())

	testutils.RequireStatus(t, recorder, http.StatusOK)
	body := testutils.DecodeJSON[dto.AuthResponse](t, recorder)
	if body.AccessToken == "" {
		t.Fatal("access token is empty")
	}
}

func TestRefreshWithoutCookie(t *testing.T) {
	service := &fakeAuthService{}
	engine := newAuthEngine(service)

	recorder := testutils.Perform(t, engine, http.MethodPost, "/refresh", nil, nil)

	testutils.RequireStatus(t, recorder, http.StatusUnauthorized)
	if service.refreshCalls != 0 {
		t.Fatal("the service must not be called without a cookie")
	}
}

func TestRefreshRotatesCookie(t *testing.T) {
	session := testSession()
	service := &fakeAuthService{session: session}
	engine := newAuthEngine(service)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/refresh", nil)
	request.AddCookie(&http.Cookie{Name: "refresh_token", Value: "old-token"})
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	testutils.RequireStatus(t, recorder, http.StatusOK)
	if service.refreshInput != "old-token" {
		t.Fatalf("service received %q", service.refreshInput)
	}
	if cookie := refreshCookie(t, recorder); cookie.Value != "refresh-token-1" {
		t.Fatalf("rotated cookie = %q", cookie.Value)
	}

	body := testutils.DecodeJSON[dto.RefreshResponse](t, recorder)
	if body.AccessToken != "access-token" {
		t.Fatalf("access token = %q", body.AccessToken)
	}
}

func TestRefreshClearsCookieOnRejection(t *testing.T) {
	service := &fakeAuthService{authErr: rules.ErrUnauthenticated}
	engine := newAuthEngine(service)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/refresh", nil)
	request.AddCookie(&http.Cookie{Name: "refresh_token", Value: "stolen-token"})
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	testutils.RequireStatus(t, recorder, http.StatusUnauthorized)
	if cookie := refreshCookie(t, recorder); cookie.MaxAge >= 0 {
		t.Fatalf("cookie MaxAge = %d, want it cleared", cookie.MaxAge)
	}
}

func TestLogoutRevokesTokenAndClearsCookie(t *testing.T) {
	service := &fakeAuthService{}
	engine := newAuthEngine(service)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/logout", nil)
	request.AddCookie(&http.Cookie{Name: "refresh_token", Value: "active-token"})
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	testutils.RequireStatus(t, recorder, http.StatusNoContent)
	if service.logoutInput != "active-token" {
		t.Fatalf("service received %q", service.logoutInput)
	}
	if cookie := refreshCookie(t, recorder); cookie.MaxAge >= 0 {
		t.Fatal("the refresh cookie must be cleared")
	}
}

func TestLogoutWithoutCookieIsIdempotent(t *testing.T) {
	service := &fakeAuthService{}
	engine := newAuthEngine(service)

	recorder := testutils.Perform(t, engine, http.MethodPost, "/logout", nil, nil)

	testutils.RequireStatus(t, recorder, http.StatusNoContent)
	if service.logoutCalls != 1 || service.logoutInput != "" {
		t.Fatalf("logout calls = %d, input = %q", service.logoutCalls, service.logoutInput)
	}
}

func TestMeReturnsUser(t *testing.T) {
	service := &fakeAuthService{user: models.User{ID: testUserID, Email: "user@example.com", CreatedAt: testCreatedAt}}
	engine := newMeEngine(service, fakeIssuer{userID: testUserID})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/me", nil)
	request.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	testutils.RequireStatus(t, recorder, http.StatusOK)
	body := testutils.DecodeJSON[dto.UserResponse](t, recorder)
	if body.ID != testUserID || body.Email != "user@example.com" {
		t.Fatalf("user = %+v", body)
	}
}

func TestMeMapsMissingUserToUnauthenticated(t *testing.T) {
	service := &fakeAuthService{meErr: rules.ErrNotFound}
	engine := newMeEngine(service, fakeIssuer{userID: testUserID})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/me", nil)
	request.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	testutils.RequireStatus(t, recorder, http.StatusUnauthorized)
	body := testutils.DecodeJSON[dto.ErrorResponse](t, recorder)
	if body.Error.Code != dto.CodeUnauthenticated {
		t.Fatalf("code = %q", body.Error.Code)
	}
}

func jsonHeader() http.Header {
	header := http.Header{}
	header.Set("Content-Type", "application/json")
	return header
}

func refreshCookie(t *testing.T, recorder *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()

	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == "refresh_token" {
			return cookie
		}
	}
	t.Fatal("refresh_token cookie is missing")
	return nil
}
