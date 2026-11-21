package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
)

// bindJSON decodes the request body and rejects malformed JSON or unknown
// fields: the API contract is closed, typos must not be silently ignored.
func bindJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		dto.Abort(c, dto.CodeValidationError, "Некорректный формат запроса.")
		return false
	}
	return true
}
