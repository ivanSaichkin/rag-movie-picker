package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/ports"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server/middleware"
)

// UserMovieHandler serves the personal lists.
type UserMovieHandler struct {
	service ports.UserMovieService
}

// NewUserMovieHandler builds the personal list handler.
func NewUserMovieHandler(service ports.UserMovieService) *UserMovieHandler {
	return &UserMovieHandler{service: service}
}

// List returns one page of the list for the requested status.
func (h *UserMovieHandler) List(c *gin.Context) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		dto.Abort(c, dto.CodeUnauthenticated, "Требуется авторизация.")
		return
	}

	status, err := rules.ParseUserMovieStatus(c.Query("status"))
	if err != nil {
		respondServiceError(c, err, "user movies list", "Запись не найдена.")
		return
	}

	page, ok := optionalQueryInt(c, "page")
	if !ok {
		return
	}
	pageSize, ok := optionalQueryInt(c, "page_size")
	if !ok {
		return
	}

	resolvedPage, resolvedPageSize, err := rules.NormalizePagination(page, pageSize)
	if err != nil {
		respondServiceError(c, err, "user movies list", "Запись не найдена.")
		return
	}

	result, err := h.service.List(c.Request.Context(), userID, status, resolvedPage, resolvedPageSize)
	if err != nil {
		respondServiceError(c, err, "user movies list", "Запись не найдена.")
		return
	}

	c.JSON(http.StatusOK, dto.NewUserMovieListResponse(result))
}

// Set creates or updates a list entry.
func (h *UserMovieHandler) Set(c *gin.Context) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		dto.Abort(c, dto.CodeUnauthenticated, "Требуется авторизация.")
		return
	}

	movieID, ok := movieIDParam(c)
	if !ok {
		return
	}

	var request dto.UserMovieUpdateRequest
	if !bindJSON(c, &request) {
		return
	}

	movie, err := h.service.Set(c.Request.Context(), userID, movieID, models.UserMovieUpdate{
		Status: models.UserMovieStatus(request.Status),
		Liked:  request.Liked,
	})
	if err != nil {
		respondServiceError(c, err, "user movie set", "Фильм не найден.")
		return
	}

	c.JSON(http.StatusOK, dto.NewUserMovieResponse(movie))
}

// Delete removes a list entry.
func (h *UserMovieHandler) Delete(c *gin.Context) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		dto.Abort(c, dto.CodeUnauthenticated, "Требуется авторизация.")
		return
	}

	movieID, ok := movieIDParam(c)
	if !ok {
		return
	}

	if err := h.service.Delete(c.Request.Context(), userID, movieID); err != nil {
		respondServiceError(c, err, "user movie delete", "Запись не найдена.")
		return
	}

	c.Status(http.StatusNoContent)
}

func movieIDParam(c *gin.Context) (int64, bool) {
	movieID, err := strconv.ParseInt(c.Param("movie_id"), 10, 64)
	if err != nil || movieID <= 0 {
		dto.Abort(c, dto.CodeValidationError, "Некорректный идентификатор фильма.")
		return 0, false
	}
	return movieID, true
}
