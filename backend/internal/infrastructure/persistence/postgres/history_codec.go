package postgres

import (
	"encoding/json"
	"fmt"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
)

// The stored structs below are the JSONB format of search_history. They are a
// storage contract, not domain models and not the HTTP DTO: the domain stays
// free of JSON tags, and the HTTP contract can change without rewriting rows.

type storedFilters struct {
	GenreIDs  []int64  `json:"genre_ids"`
	YearFrom  *int32   `json:"year_from,omitempty"`
	YearTo    *int32   `json:"year_to,omitempty"`
	MinRating *float64 `json:"min_rating,omitempty"`
}

type storedGenre struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type storedMovie struct {
	MovieID   int64         `json:"movie_id"`
	Title     string        `json:"title"`
	Year      *int32        `json:"year,omitempty"`
	Genres    []storedGenre `json:"genres"`
	Rating    *float64      `json:"rating,omitempty"`
	VoteCount int64         `json:"vote_count"`
	Overview  string        `json:"overview"`
}

type storedItem struct {
	Movie  storedMovie `json:"movie"`
	Reason *string     `json:"reason,omitempty"`
}

type storedWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type storedResponse struct {
	Mode         string          `json:"mode"`
	StrategyUsed string          `json:"strategy_used"`
	Items        []storedItem    `json:"items"`
	Warnings     []storedWarning `json:"warnings"`
}

// encodeStoredFilters serializes the domain filters into the column format.
func encodeStoredFilters(filters models.Filters) ([]byte, error) {
	raw, err := json.Marshal(storedFiltersFromModel(filters))
	if err != nil {
		return nil, fmt.Errorf("encode history filters: %w", err)
	}
	return raw, nil
}

// decodeStoredFilters reads the column format back into the domain filters.
func decodeStoredFilters(raw []byte) (models.Filters, error) {
	var filters storedFilters
	if err := json.Unmarshal(raw, &filters); err != nil {
		return models.Filters{}, fmt.Errorf("decode history filters: %w", err)
	}
	return storedFiltersToModel(filters), nil
}

// encodeStoredResponse serializes the recommendation snapshot.
func encodeStoredResponse(response models.Recommendation) ([]byte, error) {
	raw, err := json.Marshal(storedResponseFromModel(response))
	if err != nil {
		return nil, fmt.Errorf("encode history response: %w", err)
	}
	return raw, nil
}

// decodeStoredResponse reads the recommendation snapshot back.
func decodeStoredResponse(raw []byte) (models.Recommendation, error) {
	var response storedResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return models.Recommendation{}, fmt.Errorf("decode history response: %w", err)
	}
	return storedResponseToModel(response), nil
}

func storedFiltersFromModel(filters models.Filters) storedFilters {
	return storedFilters{
		GenreIDs:  append([]int64{}, filters.GenreIDs...),
		YearFrom:  filters.YearFrom,
		YearTo:    filters.YearTo,
		MinRating: filters.MinRating,
	}
}

func storedFiltersToModel(filters storedFilters) models.Filters {
	return models.Filters{
		GenreIDs:  append([]int64(nil), filters.GenreIDs...),
		YearFrom:  filters.YearFrom,
		YearTo:    filters.YearTo,
		MinRating: filters.MinRating,
	}
}

func storedResponseFromModel(response models.Recommendation) storedResponse {
	items := make([]storedItem, 0, len(response.Items))
	for _, item := range response.Items {
		items = append(items, storedItem{
			Movie:  storedMovieFromModel(item.Movie),
			Reason: item.Reason,
		})
	}

	warnings := make([]storedWarning, 0, len(response.Warnings))
	for _, warning := range response.Warnings {
		warnings = append(warnings, storedWarning{Code: warning.Code, Message: warning.Message})
	}

	return storedResponse{
		Mode:         response.Mode,
		StrategyUsed: response.StrategyUsed,
		Items:        items,
		Warnings:     warnings,
	}
}

func storedResponseToModel(response storedResponse) models.Recommendation {
	items := make([]models.RecommendedMovie, 0, len(response.Items))
	for _, item := range response.Items {
		items = append(items, models.RecommendedMovie{
			Movie:  storedMovieToModel(item.Movie),
			Reason: item.Reason,
		})
	}

	warnings := make([]models.Warning, 0, len(response.Warnings))
	for _, warning := range response.Warnings {
		warnings = append(warnings, models.Warning{Code: warning.Code, Message: warning.Message})
	}

	return models.Recommendation{
		Mode:         response.Mode,
		StrategyUsed: response.StrategyUsed,
		Items:        items,
		Warnings:     warnings,
	}
}

func storedMovieFromModel(movie models.MovieCard) storedMovie {
	genres := make([]storedGenre, 0, len(movie.Genres))
	for _, genre := range movie.Genres {
		genres = append(genres, storedGenre{ID: genre.ID, Name: genre.Name})
	}
	return storedMovie{
		MovieID:   movie.MovieID,
		Title:     movie.Title,
		Year:      movie.Year,
		Genres:    genres,
		Rating:    movie.Rating,
		VoteCount: movie.VoteCount,
		Overview:  movie.Overview,
	}
}

func storedMovieToModel(movie storedMovie) models.MovieCard {
	genres := make([]models.Genre, 0, len(movie.Genres))
	for _, genre := range movie.Genres {
		genres = append(genres, models.Genre{ID: genre.ID, Name: genre.Name})
	}
	return models.MovieCard{
		MovieID:   movie.MovieID,
		Title:     movie.Title,
		Year:      movie.Year,
		Genres:    genres,
		Rating:    movie.Rating,
		VoteCount: movie.VoteCount,
		Overview:  movie.Overview,
	}
}
