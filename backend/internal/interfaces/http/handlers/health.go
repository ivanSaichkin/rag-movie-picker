// Package handlers contains the HTTP handlers of the API service.
package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/ports"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server/middleware"
)

const readinessTimeout = 2 * time.Second

// HealthHandler reports process liveness and dependency readiness.
type HealthHandler struct {
	checks []ports.ReadinessCheck
}

// NewHealthHandler builds the health handler. Checks are evaluated in the given order.
func NewHealthHandler(checks ...ports.ReadinessCheck) *HealthHandler {
	return &HealthHandler{checks: checks}
}

// Liveness reports that the process is running.
func (h *HealthHandler) Liveness(c *gin.Context) {
	c.JSON(http.StatusOK, dto.HealthResponse{Status: "ok"})
}

// Readiness reports whether the service can serve traffic. Dependency errors
// are logged with the request identifier but never returned to the client.
func (h *HealthHandler) Readiness(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), readinessTimeout)
	defer cancel()

	logger := middleware.LoggerFrom(c)
	failed := make([]string, 0, len(h.checks))
	for _, check := range h.checks {
		if err := check.Check(ctx); err != nil {
			logger.Error("readiness check failed", "check", check.Name(), "error", err)
			failed = append(failed, check.Name())
		}
	}

	if len(failed) > 0 {
		c.JSON(http.StatusServiceUnavailable, dto.ReadinessResponse{
			Status:       "unavailable",
			FailedChecks: failed,
		})
		return
	}
	c.JSON(http.StatusOK, dto.ReadinessResponse{Status: "ok"})
}
