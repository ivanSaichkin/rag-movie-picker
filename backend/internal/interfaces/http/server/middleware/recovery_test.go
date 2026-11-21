package middleware_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server/middleware"
)

func TestRecoveryTurnsPanicIntoInternalError(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, &slog.HandlerOptions{Level: slog.LevelDebug}))

	engine := gin.New()
	engine.Use(middleware.RequestID(), middleware.Logging(logger), middleware.Recovery())
	engine.GET("/panic", func(*gin.Context) {
		panic("boom")
	})

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/panic", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "boom") {
		t.Fatal("response must not leak the panic value")
	}

	var body dto.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error.Code != dto.CodeInternal {
		t.Fatalf("code = %q, want %q", body.Error.Code, dto.CodeInternal)
	}
	if body.Error.Message == "" {
		t.Fatal("error message is empty")
	}

	logs := buffer.String()
	if !strings.Contains(logs, "panic recovered") {
		t.Fatalf("panic must be logged, logs = %s", logs)
	}
	if !strings.Contains(logs, "boom") {
		t.Fatalf("panic value must be logged, logs = %s", logs)
	}
}
