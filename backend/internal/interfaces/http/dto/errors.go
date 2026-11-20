// Package dto defines the HTTP request and response payloads.
package dto

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrorCode is a stable machine-readable identifier of an API error.
type ErrorCode string

// Error codes returned by the API. Each code maps to exactly one HTTP status.
const (
	CodeValidationError ErrorCode = "VALIDATION_ERROR"
	CodeUnauthenticated ErrorCode = "UNAUTHENTICATED"
	CodeTokenExpired    ErrorCode = "TOKEN_EXPIRED"
	//nolint:gosec // G101: this is an error code, not a credential.
	CodeInvalidCredentials    ErrorCode = "INVALID_CREDENTIALS"
	CodeForbidden             ErrorCode = "FORBIDDEN"
	CodeNotFound              ErrorCode = "NOT_FOUND"
	CodeMethodNotAllowed      ErrorCode = "METHOD_NOT_ALLOWED"
	CodeConflict              ErrorCode = "CONFLICT"
	CodeEmailTaken            ErrorCode = "EMAIL_TAKEN"
	CodeRateLimited           ErrorCode = "RATE_LIMITED"
	CodeDependencyUnavailable ErrorCode = "DEPENDENCY_UNAVAILABLE"
	CodeDeadlineExceeded      ErrorCode = "DEADLINE_EXCEEDED"
	CodeInternal              ErrorCode = "INTERNAL"
)

// statusByCode keeps exactly one HTTP status per error code so that clients
// can rely on the pair staying in sync.
var statusByCode = map[ErrorCode]int{
	CodeValidationError:       http.StatusUnprocessableEntity,
	CodeUnauthenticated:       http.StatusUnauthorized,
	CodeTokenExpired:          http.StatusUnauthorized,
	CodeInvalidCredentials:    http.StatusUnauthorized,
	CodeForbidden:             http.StatusForbidden,
	CodeNotFound:              http.StatusNotFound,
	CodeMethodNotAllowed:      http.StatusMethodNotAllowed,
	CodeConflict:              http.StatusConflict,
	CodeEmailTaken:            http.StatusConflict,
	CodeRateLimited:           http.StatusTooManyRequests,
	CodeDependencyUnavailable: http.StatusServiceUnavailable,
	CodeDeadlineExceeded:      http.StatusGatewayTimeout,
	CodeInternal:              http.StatusInternalServerError,
}

// Status returns the HTTP status that corresponds to the code. An unknown code
// is a programming error, so it maps to an internal error instead of leaking.
func Status(code ErrorCode) int {
	if status, ok := statusByCode[code]; ok {
		return status
	}
	return http.StatusInternalServerError
}

// ErrorResponse is the only error envelope of the API.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody describes an API error for the client.
type ErrorBody struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

// Abort stops the handler chain and writes the error envelope.
func Abort(c *gin.Context, code ErrorCode, message string) {
	c.AbortWithStatusJSON(Status(code), ErrorResponse{
		Error: ErrorBody{Code: code, Message: message},
	})
}
