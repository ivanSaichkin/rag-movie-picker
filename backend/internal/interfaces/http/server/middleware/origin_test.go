package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server/middleware"
)

const testOrigin = "http://127.0.0.1:8080"

func newOriginEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.CheckOrigin(testOrigin))
	engine.GET("/resource", func(c *gin.Context) { c.Status(http.StatusOK) })
	engine.POST("/resource", func(c *gin.Context) { c.Status(http.StatusOK) })
	return engine
}

func TestCheckOriginAllowsSameOrigin(t *testing.T) {
	engine := newOriginEngine()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/resource", nil)
	request.Header.Set("Origin", testOrigin)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
}

func TestCheckOriginRejectsForeignOrigin(t *testing.T) {
	engine := newOriginEngine()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/resource", nil)
	request.Header.Set("Origin", "https://evil.example")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	assertErrorCode(t, recorder, http.StatusForbidden, dto.CodeForbidden)
}

func TestCheckOriginAllowsRequestsWithoutOrigin(t *testing.T) {
	engine := newOriginEngine()

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/resource", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: non-browser clients send no Origin", recorder.Code)
	}
}

func TestCheckOriginIgnoresSafeMethods(t *testing.T) {
	engine := newOriginEngine()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/resource", nil)
	request.Header.Set("Origin", "https://evil.example")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: GET is not state-changing", recorder.Code)
	}
}
