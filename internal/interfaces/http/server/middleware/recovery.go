package middleware

import (
	"errors"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"github.com/ivanSaichkin/rag-movie-picker/internal/interfaces/http/dto"
)

// Recovery converts a handler panic into an internal error response. The panic
// value and the stack stay in the log: the client gets no internals.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			// net/http uses ErrAbortHandler to silently abort a response; it
			// must keep propagating to the server.
			if err, ok := recovered.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(recovered)
			}
			LoggerFrom(c).Error("panic recovered",
				"panic", recovered,
				"stack", string(debug.Stack()),
			)
			dto.Abort(c, dto.CodeInternal, "Внутренняя ошибка сервера")
		}()
		c.Next()
	}
}
