package postgres

import (
	"encoding/json"
	"fmt"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
)

// The snapshot structs keep the stored JSON stable and independent of the
// domain structs: the database format is a contract of its own.

type historyFilters struct {
	GenreIDs  []int64  `json:"genre_ids"`
	YearFrom  *int32   `json:"year_from,omitempty"`
	YearTo    *int32   `json:"year_to,omitempty"`
	MinRating *float64 `json:"min_rating,omitempty"`
}

type historyGenre struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type historyMovie struct {
	MovieID   int64          `json:"movie_id"`
	Title     string         `json:"title"`
	Year      *int32         `json:"year,omitempty"`
	Genres    []historyGenre `json:"genres"`
	Rating    *float64       `json:"rating,omitempty"`
	VoteCount int64          `json:"vote_count"`
	Overview  string         `json:"overview"`
}

type historyItem struct {
	Movie  historyMovie `json:"movie"`
	Reason *string      `json:"reason,omitempty"`
}

type historyWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type historyResponse struct {
	Mode         string           `json:"mode"`
	StrategyUsed string           `json:"strategy_used"`
	Items        []historyItem    `json:"items"`
	Warnings     []historyWarning `json:"warnings"`
}

func historyFiltersFromModel(filters models.Filters) historyFilters {
	return historyFilters{
		GenreIDs:  append([]int64(nil), filters.GenreIDs...),
		YearFrom:  filters.YearFrom,
		YearTo:    filters.YearTo,
		MinRating: filters.MinRating,
	}
}

func historyFiltersToModel(filters historyFilters) models.Filters {
	return models.Filters{
		GenreIDs:  append([]int64(nil), filters.GenreIDs...),
		YearFrom:  filters.YearFrom,
		YearTo:    filters.YearTo,
		MinRating: filters.MinRating,
	}
}

func decodeHistoryFilters(raw []byte) (models.Filters, error) {
	var filters historyFilters
	if err := json.Unmarshal(raw, &filters); err != nil {
		return models.Filters{}, fmt.Errorf("decode history filters: %w", err)
	}
	return historyFiltersToModel(filters), nil
}

func historyResponseFromModel(response models.Recommendation) historyResponse {
	items := make([]historyItem, 0, len(response.Items))
	for _, item := range response.Items {
		items = append(items, historyItem{
			Movie:  historyMovieFromModel(item.Movie),
			Reason: item.Reason,
		})
	}

	warnings := make([]historyWarning, 0, len(response.Warnings))
	for _, warning := range response.Warnings {
		warnings = append(warnings, historyWarning{Code: warning.Code, Message: warning.Message})
	}

	return historyResponse{
		Mode:         response.Mode,
		StrategyUsed: response.StrategyUsed,
		Items:        items,
		Warnings:     warnings,
	}
}

func historyResponseToModel(response historyResponse) models.Recommendation {
	items := make([]models.RecommendedMovie, 0, len(response.Items))
	for _, item := range response.Items {
		items = append(items, models.RecommendedMovie{
			Movie:  historyMovieToModel(item.Movie),
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

func decodeHistoryResponse(raw []byte) (models.Recommendation, error) {
	var response historyResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return models.Recommendation{}, fmt.Errorf("decode history response: %w", err)
	}
	return historyResponseToModel(response), nil
}

func historyMovieFromModel(movie models.MovieCard) historyMovie {
	genres := make([]historyGenre, 0, len(movie.Genres))
	for _, genre := range movie.Genres {
		genres = append(genres, historyGenre{ID: genre.ID, Name: genre.Name})
	}
	return historyMovie{
		MovieID:   movie.MovieID,
		Title:     movie.Title,
		Year:      movie.Year,
		Genres:    genres,
		Rating:    movie.Rating,
		VoteCount: movie.VoteCount,
		Overview:  movie.Overview,
	}
}

func historyMovieToModel(movie historyMovie) models.MovieCard {
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
