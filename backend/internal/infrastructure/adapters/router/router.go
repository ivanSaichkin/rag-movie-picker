// Package router builds the HTTP router of the API service.
package router

import (
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/handlers"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server/middleware"
)

// Limiters holds the per-route rate limiting middleware. A nil limiter is
// skipped, which keeps router tests free of cache setup.
type Limiters struct {
	Login    gin.HandlerFunc
	Register gin.HandlerFunc
	Refresh  gin.HandlerFunc
}

// Params describes the handlers and middleware registered by the router.
type Params struct {
	Logger        *slog.Logger
	Health        *handlers.HealthHandler
	Auth          *handlers.AuthHandler
	Authenticate  gin.HandlerFunc
	AllowedOrigin string
	Limiters      Limiters
}

// New assembles the middleware chain, the error handlers and the routes.
func New(params Params) *gin.Engine {
	if os.Getenv(gin.EnvGinMode) == "" {
		gin.SetMode(gin.ReleaseMode)
	}
	// Unknown JSON fields are rejected: the API contract is closed.
	binding.EnableDecoderDisallowUnknownFields = true

	engine := gin.New()
	engine.HandleMethodNotAllowed = true
	// Redirects would turn a typo in the path into a 301 instead of an
	// explicit error, so trailing-slash and fixed-path redirects are off.
	engine.RedirectTrailingSlash = false
	engine.RedirectFixedPath = false

	engine.Use(
		middleware.RequestID(),
		middleware.Logging(params.Logger),
		middleware.Recovery(),
		middleware.CheckOrigin(params.AllowedOrigin),
	)

	engine.NoRoute(func(c *gin.Context) {
		dto.Abort(c, dto.CodeNotFound, "Ресурс не найден")
	})
	engine.NoMethod(func(c *gin.Context) {
		dto.Abort(c, dto.CodeMethodNotAllowed, "Метод не поддерживается")
	})

	engine.GET("/health", params.Health.Liveness)
	engine.GET("/ready", params.Health.Readiness)

	v1 := engine.Group("/api/v1")
	auth := v1.Group("/auth")
	auth.POST("/register", with(params.Limiters.Register, params.Auth.Register)...)
	auth.POST("/login", with(params.Limiters.Login, params.Auth.Login)...)
	auth.POST("/refresh", with(params.Limiters.Refresh, params.Auth.Refresh)...)
	auth.POST("/logout", params.Auth.Logout)
	auth.GET("/me", with(params.Authenticate, params.Auth.CurrentUser)...)

	return engine
}

// with skips nil middleware so that the same registration works in tests.
func with(handlers ...gin.HandlerFunc) []gin.HandlerFunc {
	kept := make([]gin.HandlerFunc, 0, len(handlers))
	for _, handler := range handlers {
		if handler != nil {
			kept = append(kept, handler)
		}
	}
	return kept
}
