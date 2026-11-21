package postgres

import (
	"encoding/json"
	"fmt"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
)

// storedMovieCard is the JSONB movie snapshot used by history items and the
// personal list. It is a storage contract, not a domain model: the domain
// stays free of JSON tags.

type storedGenre struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type storedMovieCard struct {
	MovieID   int64         `json:"movie_id"`
	Title     string        `json:"title"`
	Year      *int32        `json:"year,omitempty"`
	Genres    []storedGenre `json:"genres"`
	Rating    *float64      `json:"rating,omitempty"`
	VoteCount int64         `json:"vote_count"`
	Overview  string        `json:"overview"`
}

func storedMovieCardFromModel(movie models.MovieCard) storedMovieCard {
	genres := make([]storedGenre, 0, len(movie.Genres))
	for _, genre := range movie.Genres {
		genres = append(genres, storedGenre{ID: genre.ID, Name: genre.Name})
	}
	return storedMovieCard{
		MovieID:   movie.MovieID,
		Title:     movie.Title,
		Year:      movie.Year,
		Genres:    genres,
		Rating:    movie.Rating,
		VoteCount: movie.VoteCount,
		Overview:  movie.Overview,
	}
}

func storedMovieCardToModel(movie storedMovieCard) models.MovieCard {
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

// encodeMovieSnapshot serializes a card for a JSONB column.
func encodeMovieSnapshot(movie models.MovieCard) ([]byte, error) {
	raw, err := json.Marshal(storedMovieCardFromModel(movie))
	if err != nil {
		return nil, fmt.Errorf("encode movie snapshot: %w", err)
	}
	return raw, nil
}

// decodeMovieSnapshot reads a card from a JSONB column.
func decodeMovieSnapshot(raw []byte) (models.MovieCard, error) {
	var movie storedMovieCard
	if err := json.Unmarshal(raw, &movie); err != nil {
		return models.MovieCard{}, fmt.Errorf("decode movie snapshot: %w", err)
	}
	return storedMovieCardToModel(movie), nil
}
