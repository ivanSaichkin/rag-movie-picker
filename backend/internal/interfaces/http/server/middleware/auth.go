package middleware

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/ports"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
)

const (
	userIDKey     = "user_id"
	bearerPrefix  = "Bearer "
	headerAuthKey = "Authorization"
)

// Authenticate verifies the access token and stores the user identifier in the
// request context.
func Authenticate(issuer ports.AccessTokenIssuer) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, found := strings.CutPrefix(c.GetHeader(headerAuthKey), bearerPrefix)
		if !found || raw == "" {
			dto.Abort(c, dto.CodeUnauthenticated, "Требуется авторизация.")
			return
		}

		userID, err := issuer.ParseAccessToken(raw)
		if err != nil {
			if errors.Is(err, rules.ErrTokenExpired) {
				dto.Abort(c, dto.CodeTokenExpired, "Срок действия токена истёк.")
				return
			}
			dto.Abort(c, dto.CodeUnauthenticated, "Требуется авторизация.")
			return
		}

		c.Set(userIDKey, userID)
		c.Next()
	}
}

// UserIDFrom returns the identifier stored by Authenticate.
func UserIDFrom(c *gin.Context) (uuid.UUID, bool) {
	value, ok := c.Get(userIDKey)
	if !ok {
		return uuid.Nil, false
	}
	userID, ok := value.(uuid.UUID)
	return userID, ok
}
