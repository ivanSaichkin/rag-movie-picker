// Package middleware contains the HTTP middleware of the API service.
package middleware

import (
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/platform/requestid"
)

// HeaderRequestID carries the correlation identifier of a request.
const HeaderRequestID = "X-Request-ID"

const requestIDKey = "request_id"

// Upstream identifiers are reused for log correlation, but only when they look
// like identifiers; anything else is replaced to keep logs and headers clean.
var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)

// RequestID reuses a valid upstream identifier or generates a new one.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderRequestID)
		if !requestIDPattern.MatchString(id) {
			id = uuid.NewString()
		}
		c.Set(requestIDKey, id)
		c.Writer.Header().Set(HeaderRequestID, id)
		c.Request = c.Request.WithContext(requestid.WithID(c.Request.Context(), id))
		c.Next()
	}
}

// RequestIDFrom returns the identifier assigned by RequestID.
func RequestIDFrom(c *gin.Context) string {
	value, ok := c.Get(requestIDKey)
	if !ok {
		return ""
	}
	id, _ := value.(string)
	return id
}
