package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
)

const headerOrigin = "Origin"

// CheckOrigin rejects state-changing requests from another origin. Requests
// without the header are non-browser clients and are allowed: the access token
// is not sent automatically, so CSRF does not apply to them.
func CheckOrigin(allowedOrigin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}

		origin := c.GetHeader(headerOrigin)
		if origin == "" {
			c.Next()
			return
		}
		if origin != allowedOrigin {
			dto.Abort(c, dto.CodeForbidden, "Запрос с недопустимого источника.")
			return
		}
		c.Next()
	}
}
