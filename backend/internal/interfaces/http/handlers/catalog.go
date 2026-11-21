package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/ports"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
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

// respondError maps RAG failures to the API envelope.
func (h *CatalogHandler) respondError(c *gin.Context, err error, action string) {
	respondServiceError(c, err, action, "Фильм не найден.")
}
