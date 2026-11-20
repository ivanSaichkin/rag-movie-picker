// Package testutils contains helpers shared by unit tests.
package testutils

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Perform sends a request through handler and returns the recorded response.
func Perform(t *testing.T, handler http.Handler, method, target string, body io.Reader, header http.Header) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequestWithContext(context.Background(), method, target, body)
	for name, values := range header {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// RequireStatus fails the test unless the recorder holds want.
func RequireStatus(t *testing.T, recorder *httptest.ResponseRecorder, want int) {
	t.Helper()

	if recorder.Code != want {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, want, recorder.Body.String())
	}
}

// DecodeJSON decodes the response body into T.
func DecodeJSON[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()

	var value T
	if err := json.NewDecoder(recorder.Body).Decode(&value); err != nil {
		t.Fatalf("decode response body: %v, body = %s", err, recorder.Body.String())
	}
	return value
}
