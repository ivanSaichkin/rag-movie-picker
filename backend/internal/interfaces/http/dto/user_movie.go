package dto

import (
	"time"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
)

// UserMovieUpdateRequest is the body of PUT /my/movies/{id}.
type UserMovieUpdateRequest struct {
	Status string `json:"status"`
	Liked  *bool  `json:"liked"`
}

// UserMovieResponse is a movie in the personal list: a flat card plus the list
// state. Liked is null when the movie has no rating.
type UserMovieResponse struct {
	MovieResponse
	Status    string    `json:"status"`
	Liked     *bool     `json:"liked"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UserMovieListResponse is one page of the personal list.
type UserMovieListResponse struct {
	Items    []UserMovieResponse `json:"items"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
	Total    int64               `json:"total"`
}

// NewUserMovieResponse converts one list entry.
func NewUserMovieResponse(movie models.UserMovie) UserMovieResponse {
	return UserMovieResponse{
		MovieResponse: NewMovieResponse(movie.Movie),
		Status:        string(movie.Status),
		Liked:         movie.Liked,
		UpdatedAt:     movie.UpdatedAt,
	}
}

// NewUserMovieListResponse converts one page of the list.
func NewUserMovieListResponse(page models.UserMoviePage) UserMovieListResponse {
	items := make([]UserMovieResponse, 0, len(page.Items))
	for _, movie := range page.Items {
		items = append(items, NewUserMovieResponse(movie))
	}
	return UserMovieListResponse{
		Items:    items,
		Page:     page.Page,
		PageSize: page.PageSize,
		Total:    page.Total,
	}
}
