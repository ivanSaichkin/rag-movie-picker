package middleware_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ivanSaichkin/rag-movie-picker/internal/interfaces/http/server/middleware"
)

func TestLoggingWritesStructuredRecord(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, &slog.HandlerOptions{Level: slog.LevelDebug}))

	engine := gin.New()
	engine.Use(middleware.RequestID(), middleware.Logging(logger))
	engine.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ping", nil)
	request.Header.Set(middleware.HeaderRequestID, "req-1")
	engine.ServeHTTP(recorder, request)

	record := decodeRecord(t, buffer.Bytes())
	if record["msg"] != "http request" {
		t.Fatalf("msg = %v", record["msg"])
	}
	if record["request_id"] != "req-1" {
		t.Fatalf("request_id = %v", record["request_id"])
	}
	if record["method"] != http.MethodGet {
		t.Fatalf("method = %v", record["method"])
	}
	if record["path"] != "/ping" {
		t.Fatalf("path = %v", record["path"])
	}
	if record["status"] != float64(http.StatusNoContent) {
		t.Fatalf("status = %v", record["status"])
	}
	if _, ok := record["duration_ms"]; !ok {
		t.Fatal("duration_ms is missing")
	}
	if record["client_ip"] == "" {
		t.Fatal("client_ip is missing")
	}
}

func TestLoggingUsesErrorLevelForServerErrors(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, &slog.HandlerOptions{Level: slog.LevelDebug}))

	engine := gin.New()
	engine.Use(middleware.RequestID(), middleware.Logging(logger))
	engine.GET("/boom", func(c *gin.Context) {
		c.Status(http.StatusInternalServerError)
	})

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/boom", nil))

	record := decodeRecord(t, buffer.Bytes())
	if record["level"] != "ERROR" {
		t.Fatalf("level = %v, want ERROR", record["level"])
	}
}

func decodeRecord(t *testing.T, data []byte) map[string]any {
	t.Helper()

	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("decode log record: %v, data = %s", err, data)
	}
	return record
}
