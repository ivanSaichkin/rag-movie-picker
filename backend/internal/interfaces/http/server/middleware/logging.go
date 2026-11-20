package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

const loggerKey = "logger"

// Logging writes one structured record per request and exposes a
// request-scoped logger to the handlers and the recovery middleware.
func Logging(base *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		logger := base.With("request_id", RequestIDFrom(c))
		c.Set(loggerKey, logger)
		c.Next()

		status := c.Writer.Status()
		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", status,
			"duration_ms", time.Since(start).Milliseconds(),
			"bytes", c.Writer.Size(),
			"client_ip", c.ClientIP(),
		}
		switch {
		case status >= 500:
			logger.Error("http request", attrs...)
		case status >= 400:
			logger.Warn("http request", attrs...)
		default:
			logger.Info("http request", attrs...)
		}
	}
}

// LoggerFrom returns the request-scoped logger, or the default one outside a
// request middleware chain.
func LoggerFrom(c *gin.Context) *slog.Logger {
	value, ok := c.Get(loggerKey)
	if !ok {
		return slog.Default()
	}
	logger, ok := value.(*slog.Logger)
	if !ok {
		return slog.Default()
	}
	return logger
}
