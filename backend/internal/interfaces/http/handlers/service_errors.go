package handlers

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server/middleware"
)

// respondServiceError maps domain and dependency errors to the API envelope.
// notFoundMessage is specific: a missing movie and a missing history record
// read differently for the user.
func respondServiceError(c *gin.Context, err error, action, notFoundMessage string) {
	var validationErr *rules.ValidationError
	switch {
	case errors.As(err, &validationErr):
		dto.Abort(c, dto.CodeValidationError, validationErr.Message)
	case errors.Is(err, rules.ErrNotFound):
		dto.Abort(c, dto.CodeNotFound, notFoundMessage)
	case errors.Is(err, rules.ErrInvalidArgument):
		dto.Abort(c, dto.CodeValidationError, "Сервис поиска отклонил запрос.")
	case errors.Is(err, rules.ErrDeadlineExceeded):
		dto.Abort(c, dto.CodeDeadlineExceeded, "Сервис поиска не ответил вовремя.")
	case errors.Is(err, rules.ErrUnavailable):
		dto.Abort(c, dto.CodeDependencyUnavailable, "Сервис поиска недоступен.")
	default:
		middleware.LoggerFrom(c).Error("request failed", "action", action, "error", err)
		dto.Abort(c, dto.CodeInternal, "Внутренняя ошибка сервера")
	}
}
