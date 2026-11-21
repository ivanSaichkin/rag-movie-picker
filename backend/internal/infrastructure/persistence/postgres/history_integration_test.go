//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/persistence/postgres"
)

func TestHistoryRepositoryRoundTrip(t *testing.T) {
	_, pool := newTestDatabase(t)
	ctx := context.Background()

	users := postgres.NewUserRepository(pool)
	owner, err := users.Create(ctx, models.User{Email: "history-owner@example.com", PasswordHash: "hash"})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	other, err := users.Create(ctx, models.User{Email: "history-other@example.com", PasswordHash: "hash"})
	if err != nil {
		t.Fatalf("create other user: %v", err)
	}

	repository := postgres.NewHistoryRepository(pool)

	yearFrom := int32(2000)
	filterRating := 6.5
	movieRating := 8.2
	reason := "подходит под запрос"
	response := models.Recommendation{
		Mode:         "generated",
		StrategyUsed: "hybrid",
		Items: []models.RecommendedMovie{{
			Movie: models.MovieCard{
				MovieID:   603,
				Title:     "The Matrix",
				Year:      int32Pointer(1999),
				Genres:    []models.Genre{{ID: 28, Name: "Action"}},
				Rating:    &movieRating,
				VoteCount: 24589,
				Overview:  "A hacker discovers the truth.",
			},
			Reason: &reason,
		}},
		Warnings: []models.Warning{{Code: "FEWER_RESULTS", Message: "меньше, чем просили"}},
	}

	first, err := repository.Create(ctx, models.HistoryEntry{
		UserID:         owner.ID,
		Query:          "космос",
		Filters:        models.Filters{GenreIDs: []int64{878}, YearFrom: &yearFrom, MinRating: &filterRating},
		RequestedLimit: 10,
		Response:       response,
	})
	if err != nil {
		t.Fatalf("create first entry: %v", err)
	}
	if first.ID == uuid.Nil || first.CreatedAt.IsZero() {
		t.Fatalf("created entry = %+v", first)
	}

	second, err := repository.Create(ctx, models.HistoryEntry{
		UserID:         owner.ID,
		Query:          "драма",
		RequestedLimit: 5,
		Response:       models.Recommendation{Mode: "empty"},
	})
	if err != nil {
		t.Fatalf("create second entry: %v", err)
	}

	items, total, err := repository.List(ctx, owner.ID, 10, 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("List() = %d items, total %d", len(items), total)
	}
	queries := map[string]bool{}
	for _, item := range items {
		queries[item.Query] = true
	}
	if !queries["космос"] || !queries["драма"] {
		t.Fatalf("queries = %v", queries)
	}

	paged, total, err := repository.List(ctx, owner.ID, 1, 1)
	if err != nil {
		t.Fatalf("List(offset) error = %v", err)
	}
	if total != 2 || len(paged) != 1 {
		t.Fatalf("paged List() = %d items, total %d", len(paged), total)
	}

	if _, otherTotal, err := repository.List(ctx, other.ID, 10, 0); err != nil || otherTotal != 0 {
		t.Fatalf("foreign list total = %d, %v", otherTotal, err)
	}

	entry, err := repository.Get(ctx, owner.ID, first.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if entry.Query != "космос" || entry.RequestedLimit != 10 {
		t.Fatalf("entry = %+v", entry)
	}
	if len(entry.Filters.GenreIDs) != 1 || entry.Filters.GenreIDs[0] != 878 {
		t.Fatalf("filters = %+v", entry.Filters)
	}
	if entry.Filters.YearFrom == nil || *entry.Filters.YearFrom != 2000 {
		t.Fatalf("year_from = %v", entry.Filters.YearFrom)
	}
	if entry.Filters.YearTo != nil {
		t.Fatalf("year_to must stay absent, got %v", *entry.Filters.YearTo)
	}
	if entry.Filters.MinRating == nil || *entry.Filters.MinRating != filterRating {
		t.Fatalf("min_rating = %v", entry.Filters.MinRating)
	}

	if entry.Response.Mode != "generated" || entry.Response.StrategyUsed != "hybrid" {
		t.Fatalf("response = %+v", entry.Response)
	}
	if len(entry.Response.Items) != 1 {
		t.Fatalf("items = %+v", entry.Response.Items)
	}
	item := entry.Response.Items[0]
	if item.Movie.MovieID != 603 || item.Movie.Title != "The Matrix" {
		t.Fatalf("movie = %+v", item.Movie)
	}
	if item.Movie.Year == nil || *item.Movie.Year != 1999 {
		t.Fatalf("movie year = %v", item.Movie.Year)
	}
	if item.Movie.Rating == nil || *item.Movie.Rating != 8.2 {
		t.Fatalf("movie rating = %v", item.Movie.Rating)
	}
	if item.Reason == nil || *item.Reason != reason {
		t.Fatalf("reason = %v", item.Reason)
	}
	if len(entry.Response.Warnings) != 1 || entry.Response.Warnings[0].Code != "FEWER_RESULTS" {
		t.Fatalf("warnings = %+v", entry.Response.Warnings)
	}

	if _, err := repository.Get(ctx, other.ID, first.ID); !errors.Is(err, rules.ErrNotFound) {
		t.Fatalf("Get() of a foreign entry error = %v, want ErrNotFound", err)
	}

	deleted, err := repository.Delete(ctx, other.ID, first.ID)
	if err != nil || deleted {
		t.Fatalf("foreign Delete() = %t, %v; want false, nil", deleted, err)
	}

	deleted, err = repository.Delete(ctx, owner.ID, first.ID)
	if err != nil || !deleted {
		t.Fatalf("Delete() = %t, %v; want true, nil", deleted, err)
	}
	if _, err := repository.Get(ctx, owner.ID, first.ID); !errors.Is(err, rules.ErrNotFound) {
		t.Fatalf("Get() after delete error = %v, want ErrNotFound", err)
	}
	if _, err := repository.Get(ctx, owner.ID, second.ID); err != nil {
		t.Fatalf("second entry must survive: %v", err)
	}
}

func int32Pointer(value int32) *int32 {
	return &value
}
