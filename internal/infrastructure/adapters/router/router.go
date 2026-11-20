// Package router builds the HTTP router of the API service.
package router

import (
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/ivanSaichkin/rag-movie-picker/internal/interfaces/http/dto"
	"github.com/ivanSaichkin/rag-movie-picker/internal/interfaces/http/handlers"
	"github.com/ivanSaichkin/rag-movie-picker/internal/interfaces/http/server/middleware"
)

// New assembles the middleware chain, the error handlers and the routes.
func New(logger *slog.Logger, health *handlers.Health) *gin.Engine {
	if os.Getenv(gin.EnvGinMode) == "" {
		gin.SetMode(gin.ReleaseMode)
	}
	engine := gin.New()
	engine.HandleMethodNotAllowed = true
	// Redirects would turn a typo in the path into a 301 instead of an
	// explicit error, so trailing-slash and fixed-path redirects are off.
	engine.RedirectTrailingSlash = false
	engine.RedirectFixedPath = false

	engine.Use(
		middleware.RequestID(),
		middleware.Logging(logger),
		middleware.Recovery(),
	)

	engine.NoRoute(func(c *gin.Context) {
		dto.Abort(c, dto.CodeNotFound, "Ресурс не найден")
	})
	engine.NoMethod(func(c *gin.Context) {
		dto.Abort(c, dto.CodeMethodNotAllowed, "Метод не поддерживается")
	})

	engine.GET("/health", health.Liveness)
	engine.GET("/ready", health.Readiness)

	return engine
}
