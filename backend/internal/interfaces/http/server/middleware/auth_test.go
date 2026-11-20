package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server/middleware"
)

type fakeIssuer struct {
	userID uuid.UUID
	err    error
}

func (f fakeIssuer) IssueAccessToken(uuid.UUID) (string, int, error) { return "", 0, nil }

func (f fakeIssuer) ParseAccessToken(string) (uuid.UUID, error) { return f.userID, f.err }

func newAuthEngine(issuer fakeIssuer) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.Authenticate(issuer))
	engine.GET("/whoami", func(c *gin.Context) {
		userID, ok := middleware.UserIDFrom(c)
		if !ok {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": userID.String()})
	})
	return engine
}

func TestAuthenticateAcceptsValidToken(t *testing.T) {
	userID := uuid.New()
	engine := newAuthEngine(fakeIssuer{userID: userID})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/whoami", nil)
	request.Header.Set("Authorization", "Bearer signed-token")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}

	var body struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.ID != userID.String() {
		t.Fatalf("user id = %q, want %q", body.ID, userID)
	}
}

func TestAuthenticateRequiresHeader(t *testing.T) {
	engine := newAuthEngine(fakeIssuer{userID: uuid.New()})

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/whoami", nil))

	assertErrorCode(t, recorder, http.StatusUnauthorized, dto.CodeUnauthenticated)
}

func TestAuthenticateReportsExpiredToken(t *testing.T) {
	engine := newAuthEngine(fakeIssuer{err: rules.ErrTokenExpired})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/whoami", nil)
	request.Header.Set("Authorization", "Bearer expired")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	assertErrorCode(t, recorder, http.StatusUnauthorized, dto.CodeTokenExpired)
}

func TestAuthenticateRejectsInvalidToken(t *testing.T) {
	engine := newAuthEngine(fakeIssuer{err: rules.ErrUnauthenticated})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/whoami", nil)
	request.Header.Set("Authorization", "Token nope")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	assertErrorCode(t, recorder, http.StatusUnauthorized, dto.CodeUnauthenticated)
}

func assertErrorCode(t *testing.T, recorder *httptest.ResponseRecorder, status int, code dto.ErrorCode) {
	t.Helper()

	if recorder.Code != status {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, status, recorder.Body.String())
	}
	var body dto.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error.Code != code {
		t.Fatalf("code = %q, want %q", body.Error.Code, code)
	}
}
