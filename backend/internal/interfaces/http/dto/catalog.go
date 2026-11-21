package dto

import "github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"

// GenreResponse is a movie genre in the API contract.
type GenreResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// CatalogResponse is the search metadata returned by GET /catalog. Optional
// bounds are omitted when the RAG service does not report them.
type CatalogResponse struct {
	Genres              []GenreResponse `json:"genres"`
	MovieCount          int64           `json:"movie_count"`
	YearFrom            *int32          `json:"year_from,omitempty"`
	YearTo              *int32          `json:"year_to,omitempty"`
	DefaultResults      int             `json:"default_results"`
	MaxResults          int             `json:"max_results"`
	AvailableStrategies []string        `json:"available_strategies"`
	DefaultStrategy     string          `json:"default_strategy"`
}

// MovieResponse is a movie card. Unknown year and rating are omitted.
type MovieResponse struct {
	MovieID   int64           `json:"movie_id"`
	Title     string          `json:"title"`
	Year      *int32          `json:"year,omitempty"`
	Genres    []GenreResponse `json:"genres"`
	Rating    *float64        `json:"rating,omitempty"`
	VoteCount int64           `json:"vote_count"`
	Overview  string          `json:"overview"`
}

// NewCatalogResponse converts the domain catalog to the API contract.
func NewCatalogResponse(catalog models.Catalog) CatalogResponse {
	return CatalogResponse{
		Genres:              newGenreResponses(catalog.Genres),
		MovieCount:          catalog.MovieCount,
		YearFrom:            catalog.YearFrom,
		YearTo:              catalog.YearTo,
		DefaultResults:      catalog.DefaultResults,
		MaxResults:          catalog.MaxResults,
		AvailableStrategies: append([]string(nil), catalog.AvailableStrategies...),
		DefaultStrategy:     catalog.DefaultStrategy,
	}
}

// NewMovieResponse converts the domain movie card to the API contract.
func NewMovieResponse(movie models.MovieCard) MovieResponse {
	return MovieResponse{
		MovieID:   movie.MovieID,
		Title:     movie.Title,
		Year:      movie.Year,
		Genres:    newGenreResponses(movie.Genres),
		Rating:    movie.Rating,
		VoteCount: movie.VoteCount,
		Overview:  movie.Overview,
	}
}

func newGenreResponses(genres []models.Genre) []GenreResponse {
	result := make([]GenreResponse, 0, len(genres))
	for _, genre := range genres {
		result = append(result, GenreResponse{ID: genre.ID, Name: genre.Name})
	}
	return result
}
