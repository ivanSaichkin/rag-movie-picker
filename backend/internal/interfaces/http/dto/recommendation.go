package dto

import (
	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
)

// RecommendationFilters is the filter block of a search request.
type RecommendationFilters struct {
	GenreIDs  []int64  `json:"genre_ids"`
	YearFrom  *int32   `json:"year_from,omitempty"`
	YearTo    *int32   `json:"year_to,omitempty"`
	MinRating *float64 `json:"min_rating,omitempty"`
}

// RecommendationRequest is the body of POST /recommendations.
type RecommendationRequest struct {
	Query                string                `json:"query"`
	Limit                *int                  `json:"limit,omitempty"`
	Filters              RecommendationFilters `json:"filters"`
	Strategy             *string               `json:"strategy,omitempty"`
	GenerateExplanations *bool                 `json:"generate_explanations,omitempty"`
	DisableLLM           bool                  `json:"disable_llm,omitempty"`
}

// ToModel converts the request to the domain form.
func (r RecommendationRequest) ToModel() models.RecommendationRequest {
	return models.RecommendationRequest{
		Query:                r.Query,
		Limit:                r.Limit,
		Filters:              newFiltersModel(r.Filters),
		Strategy:             r.Strategy,
		GenerateExplanations: r.GenerateExplanations,
		DisableLLM:           r.DisableLLM,
	}
}

// WarningResponse is one warning entry.
type WarningResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// RecommendationItem is a flat movie card with an optional explanation.
type RecommendationItem struct {
	MovieResponse
	Reason *string `json:"reason,omitempty"`
}

// StoredRecommendation is the search answer that is also kept in the history.
type StoredRecommendation struct {
	Mode         string               `json:"mode"`
	StrategyUsed string               `json:"strategy_used"`
	Items        []RecommendationItem `json:"items"`
	Warnings     []WarningResponse    `json:"warnings"`
}

// RecommendationResponse is the answer of POST /recommendations. The history
// identifier is null when the snapshot could not be stored.
type RecommendationResponse struct {
	HistoryID *uuid.UUID `json:"history_id"`
	StoredRecommendation
}

// NewRecommendationResponse converts the service result to the API contract.
func NewRecommendationResponse(result models.RecommendationResult) RecommendationResponse {
	return RecommendationResponse{
		HistoryID:            result.HistoryID,
		StoredRecommendation: newStoredRecommendation(result.Response),
	}
}

func newStoredRecommendation(response models.Recommendation) StoredRecommendation {
	items := make([]RecommendationItem, 0, len(response.Items))
	for _, item := range response.Items {
		items = append(items, RecommendationItem{
			MovieResponse: NewMovieResponse(item.Movie),
			Reason:        item.Reason,
		})
	}

	warnings := make([]WarningResponse, 0, len(response.Warnings))
	for _, warning := range response.Warnings {
		warnings = append(warnings, WarningResponse{Code: warning.Code, Message: warning.Message})
	}

	return StoredRecommendation{
		Mode:         response.Mode,
		StrategyUsed: response.StrategyUsed,
		Items:        items,
		Warnings:     warnings,
	}
}

func newRecommendationFilters(filters models.Filters) RecommendationFilters {
	return RecommendationFilters{
		GenreIDs:  append([]int64{}, filters.GenreIDs...),
		YearFrom:  filters.YearFrom,
		YearTo:    filters.YearTo,
		MinRating: filters.MinRating,
	}
}

func newFiltersModel(filters RecommendationFilters) models.Filters {
	return models.Filters{
		GenreIDs:  append([]int64(nil), filters.GenreIDs...),
		YearFrom:  filters.YearFrom,
		YearTo:    filters.YearTo,
		MinRating: filters.MinRating,
	}
}
