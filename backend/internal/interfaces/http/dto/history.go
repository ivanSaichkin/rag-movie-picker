package dto

import (
	"time"

	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
)

// HistoryItemResponse is a short history record used by the history page.
type HistoryItemResponse struct {
	HistoryID      uuid.UUID             `json:"history_id"`
	Query          string                `json:"query"`
	Filters        RecommendationFilters `json:"filters"`
	RequestedLimit int                   `json:"requested_limit"`
	CreatedAt      time.Time             `json:"created_at"`
}

// HistoryListResponse is one page of history.
type HistoryListResponse struct {
	Items    []HistoryItemResponse `json:"items"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"page_size"`
	Total    int64                 `json:"total"`
}

// HistoryEntryResponse is the full stored snapshot.
type HistoryEntryResponse struct {
	HistoryID      uuid.UUID             `json:"history_id"`
	Query          string                `json:"query"`
	Filters        RecommendationFilters `json:"filters"`
	RequestedLimit int                   `json:"requested_limit"`
	Response       StoredRecommendation  `json:"response"`
	CreatedAt      time.Time             `json:"created_at"`
}

// NewHistoryListResponse converts one page of history.
func NewHistoryListResponse(page models.HistoryPage) HistoryListResponse {
	items := make([]HistoryItemResponse, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, HistoryItemResponse{
			HistoryID:      item.ID,
			Query:          item.Query,
			Filters:        newRecommendationFilters(item.Filters),
			RequestedLimit: item.RequestedLimit,
			CreatedAt:      item.CreatedAt,
		})
	}

	return HistoryListResponse{
		Items:    items,
		Page:     page.Page,
		PageSize: page.PageSize,
		Total:    page.Total,
	}
}

// NewHistoryEntryResponse converts one stored snapshot.
func NewHistoryEntryResponse(entry models.HistoryEntry) HistoryEntryResponse {
	return HistoryEntryResponse{
		HistoryID:      entry.ID,
		Query:          entry.Query,
		Filters:        newRecommendationFilters(entry.Filters),
		RequestedLimit: entry.RequestedLimit,
		Response:       newStoredRecommendation(entry.Response),
		CreatedAt:      entry.CreatedAt,
	}
}
