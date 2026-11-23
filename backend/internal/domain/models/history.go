package models

import (
	"time"

	"github.com/google/uuid"
)

// HistoryEntry is a stored recommendation snapshot. The response is stored as
// is: opening history never calls RAG again.
type HistoryEntry struct {
	ChatID         *uuid.UUID
	ID             uuid.UUID
	UserID         uuid.UUID
	Query          string
	Filters        Filters
	RequestedLimit int
	Response       Recommendation
	CreatedAt      time.Time
}

// Chat summarizes the stored turns of a search conversation.
type Chat struct {
	ID        uuid.UUID
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// HistoryListItem is the short form used by the history page.
type HistoryListItem struct {
	ID             uuid.UUID
	Query          string
	Filters        Filters
	RequestedLimit int
	CreatedAt      time.Time
}

// HistoryPage is one page of history with the total count.
type HistoryPage struct {
	Items    []HistoryListItem
	Page     int
	PageSize int
	Total    int64
}
