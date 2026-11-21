package dto_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
)

func TestStatusMapping(t *testing.T) {
	cases := map[dto.ErrorCode]int{
		dto.CodeValidationError:       http.StatusUnprocessableEntity,
		dto.CodeUnauthenticated:       http.StatusUnauthorized,
		dto.CodeTokenExpired:          http.StatusUnauthorized,
		dto.CodeInvalidCredentials:    http.StatusUnauthorized,
		dto.CodeForbidden:             http.StatusForbidden,
		dto.CodeNotFound:              http.StatusNotFound,
		dto.CodeMethodNotAllowed:      http.StatusMethodNotAllowed,
		dto.CodeConflict:              http.StatusConflict,
		dto.CodeEmailTaken:            http.StatusConflict,
		dto.CodeRateLimited:           http.StatusTooManyRequests,
		dto.CodeDependencyUnavailable: http.StatusServiceUnavailable,
		dto.CodeDeadlineExceeded:      http.StatusGatewayTimeout,
		dto.CodeInternal:              http.StatusInternalServerError,
	}

	for code, want := range cases {
		if got := dto.Status(code); got != want {
			t.Fatalf("Status(%q) = %d, want %d", code, got, want)
		}
	}
}

func TestStatusUnknownCodeIsInternal(t *testing.T) {
	if got := dto.Status(dto.ErrorCode("SOMETHING_ELSE")); got != http.StatusInternalServerError {
		t.Fatalf("Status(unknown) = %d, want 500", got)
	}
}

func TestAbortWritesEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)

	dto.Abort(context, dto.CodeNotFound, "Ресурс не найден")

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
	if !context.IsAborted() {
		t.Fatal("context must be aborted to stop the handler chain")
	}

	var body dto.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error.Code != dto.CodeNotFound {
		t.Fatalf("code = %q, want %q", body.Error.Code, dto.CodeNotFound)
	}
	if body.Error.Message == "" {
		t.Fatal("message is empty")
	}
}
