package dto

// HealthResponse is returned by the liveness endpoint.
type HealthResponse struct {
	Status string `json:"status"`
}

// ReadinessResponse is returned by the readiness endpoint. Dependency errors
// are logged, so the response names failed checks without their details.
type ReadinessResponse struct {
	Status       string   `json:"status"`
	FailedChecks []string `json:"failed_checks,omitempty"`
}
