package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/ports"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server/middleware"
)

// Auth serves the authentication endpoints.
type Auth struct {
	service       ports.AuthService
	secureCookies bool
}

// NewAuth builds the authentication handler. The refresh cookie lifetime comes
// from the session itself, so the handler does not need the TTL separately.
func NewAuth(service ports.AuthService, secureCookies bool) *Auth {
	return &Auth{service: service, secureCookies: secureCookies}
}

// Register creates an account and starts a session.
func (h *Auth) Register(c *gin.Context) {
	var credentials dto.AuthCredentials
	if !bindJSON(c, &credentials) {
		return
	}

	session, err := h.service.Register(c.Request.Context(), credentials.Email, credentials.Password)
	if err != nil {
		h.respondError(c, err, "register")
		return
	}
	h.respondSession(c, http.StatusCreated, session)
}

// Login verifies the credentials and starts a session.
func (h *Auth) Login(c *gin.Context) {
	var credentials dto.AuthCredentials
	if !bindJSON(c, &credentials) {
		return
	}

	session, err := h.service.Login(c.Request.Context(), credentials.Email, credentials.Password)
	if err != nil {
		h.respondError(c, err, "login")
		return
	}
	h.respondSession(c, http.StatusOK, session)
}

// Refresh rotates the refresh token from the cookie.
func (h *Auth) Refresh(c *gin.Context) {
	refreshToken, found := refreshTokenFromCookie(c)
	if !found {
		dto.Abort(c, dto.CodeUnauthenticated, "Сессия не найдена.")
		return
	}

	session, err := h.service.Refresh(c.Request.Context(), refreshToken)
	if err != nil {
		h.respondError(c, err, "refresh")
		return
	}

	setRefreshCookie(c, session.RefreshToken, session.RefreshExpiresAt, h.secureCookies)
	c.JSON(http.StatusOK, dto.RefreshResponse{
		AccessToken: session.AccessToken,
		ExpiresIn:   session.AccessExpiresIn,
	})
}

// Logout revokes the refresh token and clears the cookie.
func (h *Auth) Logout(c *gin.Context) {
	refreshToken, _ := refreshTokenFromCookie(c)
	if err := h.service.Logout(c.Request.Context(), refreshToken); err != nil {
		h.respondError(c, err, "logout")
		return
	}

	clearRefreshCookie(c, h.secureCookies)
	c.Status(http.StatusNoContent)
}

// Me returns the account of the authenticated user.
func (h *Auth) Me(c *gin.Context) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		dto.Abort(c, dto.CodeUnauthenticated, "Требуется авторизация.")
		return
	}

	user, err := h.service.Me(c.Request.Context(), userID)
	if err != nil {
		h.respondError(c, err, "me")
		return
	}

	c.JSON(http.StatusOK, dto.UserResponse{ID: user.ID, Email: user.Email, CreatedAt: user.CreatedAt})
}

func (h *Auth) respondSession(c *gin.Context, status int, session models.Session) {
	setRefreshCookie(c, session.RefreshToken, session.RefreshExpiresAt, h.secureCookies)
	c.JSON(status, dto.AuthResponse{
		User: dto.UserResponse{
			ID:        session.User.ID,
			Email:     session.User.Email,
			CreatedAt: session.User.CreatedAt,
		},
		AccessToken: session.AccessToken,
		ExpiresIn:   session.AccessExpiresIn,
	})
}

// respondError maps domain errors to the API envelope and keeps everything else
// in the log.
func (h *Auth) respondError(c *gin.Context, err error, action string) {
	var validationErr *rules.ValidationError
	switch {
	case errors.As(err, &validationErr):
		dto.Abort(c, dto.CodeValidationError, validationErr.Message)
	case errors.Is(err, rules.ErrEmailTaken):
		dto.Abort(c, dto.CodeEmailTaken, "Адрес электронной почты уже зарегистрирован.")
	case errors.Is(err, rules.ErrInvalidCredentials):
		dto.Abort(c, dto.CodeInvalidCredentials, "Неверная пара электронной почты и пароля.")
	case errors.Is(err, rules.ErrUnauthenticated), errors.Is(err, rules.ErrTokenExpired),
		errors.Is(err, rules.ErrNotFound):
		clearRefreshCookie(c, h.secureCookies)
		dto.Abort(c, dto.CodeUnauthenticated, "Сессия недействительна.")
	default:
		middleware.LoggerFrom(c).Error("auth request failed", "action", action, "error", err)
		dto.Abort(c, dto.CodeInternal, "Внутренняя ошибка сервера")
	}
}
