package postgres

import (
	"strings"
	"testing"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
)

func TestStoredFiltersRoundTrip(t *testing.T) {
	yearFrom := int32(2000)
	rating := 6.5

	raw, err := encodeStoredFilters(models.Filters{GenreIDs: []int64{878}, YearFrom: &yearFrom, MinRating: &rating})
	if err != nil {
		t.Fatalf("encodeStoredFilters() error = %v", err)
	}
	if !strings.Contains(string(raw), `"year_from":2000`) || !strings.Contains(string(raw), `"min_rating":6.5`) {
		t.Fatalf("stored filters = %s", raw)
	}
	if strings.Contains(string(raw), `"year_to"`) {
		t.Fatalf("absent year_to must be omitted: %s", raw)
	}

	decoded, err := decodeStoredFilters(raw)
	if err != nil {
		t.Fatalf("decodeStoredFilters() error = %v", err)
	}
	if len(decoded.GenreIDs) != 1 || decoded.GenreIDs[0] != 878 {
		t.Fatalf("genre ids = %v", decoded.GenreIDs)
	}
	if decoded.YearFrom == nil || *decoded.YearFrom != 2000 {
		t.Fatalf("year_from = %v", decoded.YearFrom)
	}
	if decoded.YearTo != nil {
		t.Fatalf("year_to = %v, want nil", decoded.YearTo)
	}
	if decoded.MinRating == nil || *decoded.MinRating != 6.5 {
		t.Fatalf("min_rating = %v", decoded.MinRating)
	}
}

func TestStoredFiltersEncodeEmptyGenresAsList(t *testing.T) {
	raw, err := encodeStoredFilters(models.Filters{})
	if err != nil {
		t.Fatalf("encodeStoredFilters() error = %v", err)
	}
	if !strings.Contains(string(raw), `"genre_ids":[]`) {
		t.Fatalf("stored filters = %s, want an empty list", raw)
	}
}

func TestStoredResponseRoundTrip(t *testing.T) {
	year := int32(1999)
	rating := 8.2
	reason := "подходит под запрос"

	response := models.Recommendation{
		Mode:         "generated",
		StrategyUsed: "hybrid",
		Items: []models.RecommendedMovie{{
			Movie: models.MovieCard{
				MovieID:   603,
				Title:     "The Matrix",
				Year:      &year,
				Genres:    []models.Genre{{ID: 28, Name: "Action"}},
				Rating:    &rating,
				VoteCount: 24589,
				Overview:  "A hacker discovers the truth.",
			},
			Reason: &reason,
		}},
		Warnings: []models.Warning{{Code: "FEWER_RESULTS", Message: "меньше"}},
	}

	raw, err := encodeStoredResponse(response)
	if err != nil {
		t.Fatalf("encodeStoredResponse() error = %v", err)
	}
	if !strings.Contains(string(raw), `"strategy_used":"hybrid"`) {
		t.Fatalf("stored response = %s", raw)
	}

	decoded, err := decodeStoredResponse(raw)
	if err != nil {
		t.Fatalf("decodeStoredResponse() error = %v", err)
	}
	if decoded.Mode != "generated" || decoded.StrategyUsed != "hybrid" {
		t.Fatalf("response = %+v", decoded)
	}
	if len(decoded.Items) != 1 || decoded.Items[0].Movie.MovieID != 603 {
		t.Fatalf("items = %+v", decoded.Items)
	}
	if decoded.Items[0].Reason == nil || *decoded.Items[0].Reason != reason {
		t.Fatalf("reason = %v", decoded.Items[0].Reason)
	}
	if decoded.Items[0].Movie.Year == nil || *decoded.Items[0].Movie.Year != 1999 {
		t.Fatalf("year = %v", decoded.Items[0].Movie.Year)
	}
	if len(decoded.Warnings) != 1 || decoded.Warnings[0].Code != "FEWER_RESULTS" {
		t.Fatalf("warnings = %+v", decoded.Warnings)
	}
}

func TestStoredResponseKeepsAbsentOptionalFields(t *testing.T) {
	raw, err := encodeStoredResponse(models.Recommendation{
		Mode:  "retrieval_only",
		Items: []models.RecommendedMovie{{Movie: models.MovieCard{MovieID: 1, Title: "Untitled"}}},
	})
	if err != nil {
		t.Fatalf("encodeStoredResponse() error = %v", err)
	}

	decoded, err := decodeStoredResponse(raw)
	if err != nil {
		t.Fatalf("decodeStoredResponse() error = %v", err)
	}
	item := decoded.Items[0]
	if item.Reason != nil || item.Movie.Year != nil || item.Movie.Rating != nil {
		t.Fatalf("absent fields = reason %v, year %v, rating %v", item.Reason, item.Movie.Year, item.Movie.Rating)
	}
}

func TestStoredResponseEncodesEmptyCollections(t *testing.T) {
	raw, err := encodeStoredResponse(models.Recommendation{Mode: "empty"})
	if err != nil {
		t.Fatalf("encodeStoredResponse() error = %v", err)
	}
	if !strings.Contains(string(raw), `"items":[]`) || !strings.Contains(string(raw), `"warnings":[]`) {
		t.Fatalf("stored response = %s, want empty lists", raw)
	}
}

func TestDecodeStoredDataRejectsCorruptedJSON(t *testing.T) {
	if _, err := decodeStoredFilters([]byte("{")); err == nil {
		t.Fatal("decodeStoredFilters() must fail on broken JSON")
	}
	if _, err := decodeStoredResponse([]byte("[")); err == nil {
		t.Fatal("decodeStoredResponse() must fail on broken JSON")
	}
}
