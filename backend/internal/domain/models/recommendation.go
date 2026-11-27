package models

import "github.com/google/uuid"

// WarningHistoryNotSaved is reported when the result is returned but the
// history snapshot could not be stored.
const WarningHistoryNotSaved = "HISTORY_NOT_SAVED"

// RecommendationRequest is the validated request passed to the RAG service.
type RecommendationRequest struct {
	Query                string
	Limit                *int
	Filters              Filters
	Strategy             *string
	GenerateExplanations *bool
	DisableLLM           bool
}

// RecommendedMovie is a movie card with an optional explanation.
type RecommendedMovie struct {
	Movie  MovieCard
	Reason *string
}

// Warning is a non-fatal note from the RAG service or the backend.
type Warning struct {
	Code    string
	Message string
}

// Recommendation is the RAG answer: the mode, the strategy that actually ran,
// the selected movies and warnings.
type Recommendation struct {
	Explanation  *string
	HasMatches   *bool
	Mode         string
	StrategyUsed string
	Items        []RecommendedMovie
	Warnings     []Warning
}

// RecommendationResult is the recommendation plus the history identifier when
// the snapshot was stored.
type RecommendationResult struct {
	HistoryID *uuid.UUID
	Response  Recommendation
}
