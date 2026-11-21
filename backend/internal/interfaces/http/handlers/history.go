package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/ports"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server/middleware"
)

// HistoryHandler serves stored search history.
type HistoryHandler struct {
	service ports.HistoryService
}

// NewHistoryHandler builds the history handler.
func NewHistoryHandler(service ports.HistoryService) *HistoryHandler {
	return &HistoryHandler{service: service}
}

// List returns one page of the user's history.
func (h *HistoryHandler) List(c *gin.Context) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		dto.Abort(c, dto.CodeUnauthenticated, "Требуется авторизация.")
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
		respondServiceError(c, err, "history list", "Запись истории не найдена.")
		return
	}

	historyPage, err := h.service.List(c.Request.Context(), userID, resolvedPage, resolvedPageSize)
	if err != nil {
		respondServiceError(c, err, "history list", "Запись истории не найдена.")
		return
	}

	c.JSON(http.StatusOK, dto.NewHistoryListResponse(historyPage))
}

// Get returns one stored snapshot with the full result.
func (h *HistoryHandler) Get(c *gin.Context) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		dto.Abort(c, dto.CodeUnauthenticated, "Требуется авторизация.")
		return
	}

	historyID, ok := historyIDParam(c)
	if !ok {
		return
	}

	entry, err := h.service.Get(c.Request.Context(), userID, historyID)
	if err != nil {
		respondServiceError(c, err, "history get", "Запись истории не найдена.")
		return
	}

	c.JSON(http.StatusOK, dto.NewHistoryEntryResponse(entry))
}

// Delete removes one stored snapshot.
func (h *HistoryHandler) Delete(c *gin.Context) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		dto.Abort(c, dto.CodeUnauthenticated, "Требуется авторизация.")
		return
	}

	historyID, ok := historyIDParam(c)
	if !ok {
		return
	}

	if err := h.service.Delete(c.Request.Context(), userID, historyID); err != nil {
		respondServiceError(c, err, "history delete", "Запись истории не найдена.")
		return
	}

	c.Status(http.StatusNoContent)
}

// optionalQueryInt parses a query parameter. An absent or empty parameter is
// nil, which lets the rules apply the default.
func optionalQueryInt(c *gin.Context, name string) (*int, bool) {
	raw, found := c.GetQuery(name)
	if !found || strings.TrimSpace(raw) == "" {
		return nil, true
	}

	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		dto.Abort(c, dto.CodeValidationError, "Некорректный параметр "+name+".")
		return nil, false
	}
	return &value, true
}

func historyIDParam(c *gin.Context) (uuid.UUID, bool) {
	historyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		dto.Abort(c, dto.CodeValidationError, "Некорректный идентификатор записи.")
		return uuid.Nil, false
	}
	return historyID, true
}
