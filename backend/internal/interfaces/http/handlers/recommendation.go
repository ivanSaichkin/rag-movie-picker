package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/ports"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server/middleware"
)

// RecommendationHandler serves search requests.
type RecommendationHandler struct {
	service ports.RecommendationService
}

// NewRecommendationHandler builds the recommendation handler.
func NewRecommendationHandler(service ports.RecommendationService) *RecommendationHandler {
	return &RecommendationHandler{service: service}
}

// Recommend validates the request, runs the search and stores the snapshot.
func (h *RecommendationHandler) Recommend(c *gin.Context) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		dto.Abort(c, dto.CodeUnauthenticated, "Требуется авторизация.")
		return
	}

	var request dto.RecommendationRequest
	if !bindJSON(c, &request) {
		return
	}

	result, err := h.service.Recommend(c.Request.Context(), userID, request.ToModel())
	if err != nil {
		respondServiceError(c, err, "recommend", "Чат не найден.")
		return
	}

	c.JSON(http.StatusOK, dto.NewRecommendationResponse(result))
}

// RecommendGuest searches the catalog without creating a personal history entry.
func (h *RecommendationHandler) RecommendGuest(c *gin.Context) {
	var request dto.RecommendationRequest
	if !bindJSON(c, &request) {
		return
	}
	result, err := h.service.RecommendGuest(c.Request.Context(), request.ToModel())
	if err != nil {
		respondServiceError(c, err, "guest_recommend", "Фильм не найден.")
		return
	}
	c.JSON(http.StatusOK, dto.NewRecommendationResponse(result))
}
