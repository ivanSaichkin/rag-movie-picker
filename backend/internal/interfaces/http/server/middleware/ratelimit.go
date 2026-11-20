package middleware

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/ports"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
)

// RateLimit rejects requests that exceed limit within a fixed window per client
// IP. A failure of the cache is fail-open: availability of authentication is
// more important than the limit itself.
func RateLimit(store ports.Cache, prefix string, limit int64, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		now := time.Now()
		windowStart := now.Truncate(window)
		key := fmt.Sprintf("ratelimit:%s:%s:%d", prefix, c.ClientIP(), windowStart.Unix())

		count, err := store.Incr(c.Request.Context(), key, window)
		if err != nil {
			LoggerFrom(c).Warn("rate limit cache failed", "prefix", prefix, "error", err)
			c.Next()
			return
		}
		if count > limit {
			retryAfter := int(windowStart.Add(window).Sub(now).Seconds()) + 1
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			dto.Abort(c, dto.CodeRateLimited, "Слишком много запросов. Попробуйте позже.")
			return
		}
		c.Next()
	}
}
