package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/ports"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server/middleware"
)

// CatalogHandler serves the catalog and movie cards.
type CatalogHandler struct {
	service ports.CatalogService
}

// NewCatalogHandler builds the catalog handler.
func NewCatalogHandler(service ports.CatalogService) *CatalogHandler {
	return &CatalogHandler{service: service}
}

// Catalog returns genres, bounds and search limits.
func (h *CatalogHandler) Catalog(c *gin.Context) {
	catalog, err := h.service.Catalog(c.Request.Context())
	if err != nil {
		h.respondError(c, err, "catalog")
		return
	}
	c.JSON(http.StatusOK, dto.NewCatalogResponse(catalog))
}

// Movie returns one movie card by the path identifier.
func (h *CatalogHandler) Movie(c *gin.Context) {
	movieID, err := strconv.ParseInt(c.Param("movie_id"), 10, 64)
	if err != nil || movieID <= 0 {
		dto.Abort(c, dto.CodeValidationError, "Некорректный идентификатор фильма.")
		return
	}

	movie, err := h.service.Movie(c.Request.Context(), movieID)
	if err != nil {
		h.respondError(c, err, "movie")
		return
	}
	c.JSON(http.StatusOK, dto.NewMovieResponse(movie))
}

// respondError maps RAG failures to the API envelope: the dependency differs
// from "no such movie" and from "service is down".
func (h *CatalogHandler) respondError(c *gin.Context, err error, action string) {
	switch {
	case errors.Is(err, rules.ErrNotFound):
		dto.Abort(c, dto.CodeNotFound, "Фильм не найден.")
	case errors.Is(err, rules.ErrInvalidArgument):
		dto.Abort(c, dto.CodeValidationError, "Сервис поиска отклонил запрос.")
	case errors.Is(err, rules.ErrDeadlineExceeded):
		dto.Abort(c, dto.CodeDeadlineExceeded, "Сервис поиска не ответил вовремя.")
	case errors.Is(err, rules.ErrUnavailable):
		dto.Abort(c, dto.CodeDependencyUnavailable, "Сервис поиска недоступен.")
	default:
		middleware.LoggerFrom(c).Error("catalog request failed", "action", action, "error", err)
		dto.Abort(c, dto.CodeInternal, "Внутренняя ошибка сервера")
	}
}
