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

func TestUserMoviesRepositoryRoundTrip(t *testing.T) {
	_, pool := newTestDatabase(t)
	ctx := context.Background()

	users := postgres.NewUserRepository(pool)
	owner, err := users.Create(ctx, models.User{Email: "lists-owner@example.com", PasswordHash: "hash"})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	other, err := users.Create(ctx, models.User{Email: "lists-other@example.com", PasswordHash: "hash"})
	if err != nil {
		t.Fatalf("create other user: %v", err)
	}

	repository := postgres.NewUserMovieRepository(pool)

	year := int32(1999)
	rating := 8.2
	original := models.MovieCard{
		MovieID:   603,
		Title:     "The Matrix",
		Year:      &year,
		Genres:    []models.Genre{{ID: 28, Name: "Action"}},
		Rating:    &rating,
		VoteCount: 24589,
		Overview:  "A hacker discovers the truth.",
	}

	created, err := repository.Save(ctx, models.UserMovie{
		UserID:  owner.ID,
		MovieID: 603,
		Status:  models.UserMovieWantToWatch,
		Movie:   original,
	})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if created.Movie.Title != "The Matrix" || created.Status != models.UserMovieWantToWatch {
		t.Fatalf("created = %+v", created)
	}
	if created.Liked != nil || created.UpdatedAt.IsZero() {
		t.Fatalf("created liked = %v, updated_at = %v", created.Liked, created.UpdatedAt)
	}

	// The second write changes the status and rating, but must keep the
	// snapshot taken on the first write.
	liked := true
	updated, err := repository.Save(ctx, models.UserMovie{
		UserID:  owner.ID,
		MovieID: 603,
		Status:  models.UserMovieWatched,
		Liked:   &liked,
		Movie:   models.MovieCard{MovieID: 603, Title: "Changed"},
	})
	if err != nil {
		t.Fatalf("Save() update error = %v", err)
	}
	if updated.Status != models.UserMovieWatched || updated.Liked == nil || !*updated.Liked {
		t.Fatalf("updated = %+v", updated)
	}
	if updated.Movie.Title != "The Matrix" {
		t.Fatalf("snapshot was replaced: %+v", updated.Movie)
	}

	found, err := repository.Find(ctx, owner.ID, 603)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if found.Status != models.UserMovieWatched || found.Movie.Title != "The Matrix" {
		t.Fatalf("found = %+v", found)
	}

	wantItems, wantTotal, err := repository.List(ctx, owner.ID, models.UserMovieWantToWatch, 10, 0)
	if err != nil || wantTotal != 0 || len(wantItems) != 0 {
		t.Fatalf("want list = %d items, total %d, %v", len(wantItems), wantTotal, err)
	}

	watchedItems, watchedTotal, err := repository.List(ctx, owner.ID, models.UserMovieWatched, 10, 0)
	if err != nil || watchedTotal != 1 || len(watchedItems) != 1 {
		t.Fatalf("watched list = %d items, total %d, %v", len(watchedItems), watchedTotal, err)
	}
	if watchedItems[0].Movie.MovieID != 603 {
		t.Fatalf("watched item = %+v", watchedItems[0])
	}

	if _, err := repository.Save(ctx, models.UserMovie{
		UserID:  owner.ID,
		MovieID: 604,
		Status:  models.UserMovieWatched,
		Movie:   models.MovieCard{MovieID: 604, Title: "Alien"},
	}); err != nil {
		t.Fatalf("Save(second) error = %v", err)
	}

	firstPage, total, err := repository.List(ctx, owner.ID, models.UserMovieWatched, 1, 0)
	if err != nil {
		t.Fatalf("List(page 1) error = %v", err)
	}
	if total != 2 || len(firstPage) != 1 {
		t.Fatalf("page 1 = %d items, total %d", len(firstPage), total)
	}

	secondPage, total, err := repository.List(ctx, owner.ID, models.UserMovieWatched, 1, 1)
	if err != nil {
		t.Fatalf("List(page 2) error = %v", err)
	}
	if total != 2 || len(secondPage) != 1 {
		t.Fatalf("page 2 = %d items, total %d", len(secondPage), total)
	}
	if firstPage[0].MovieID == secondPage[0].MovieID {
		t.Fatalf("pages overlap: %d and %d", firstPage[0].MovieID, secondPage[0].MovieID)
	}

	if _, otherTotal, err := repository.List(ctx, other.ID, models.UserMovieWatched, 10, 0); err != nil || otherTotal != 0 {
		t.Fatalf("foreign list total = %d, %v", otherTotal, err)
	}
	if _, err := repository.Find(ctx, other.ID, 603); !errors.Is(err, rules.ErrNotFound) {
		t.Fatalf("foreign Find() error = %v, want ErrNotFound", err)
	}
	if deleted, err := repository.Delete(ctx, other.ID, 603); err != nil || deleted {
		t.Fatalf("foreign Delete() = %t, %v", deleted, err)
	}

	// The database must keep the rating rule even if the service is bypassed.
	if _, err := pool.Exec(ctx,
		`INSERT INTO user_movies (user_id, movie_id, status, liked, movie_snapshot) VALUES ($1, 605, 'want_to_watch', true, '{}')`,
		owner.ID,
	); err == nil {
		t.Fatal("liked without watched must be rejected by the database")
	}

	deleted, err := repository.Delete(ctx, owner.ID, 603)
	if err != nil || !deleted {
		t.Fatalf("Delete() = %t, %v", deleted, err)
	}
	if _, err := repository.Find(ctx, owner.ID, 603); !errors.Is(err, rules.ErrNotFound) {
		t.Fatalf("Find() after delete = %v, want ErrNotFound", err)
	}
	if deleted, err := repository.Delete(ctx, owner.ID, 603); err != nil || deleted {
		t.Fatalf("second Delete() = %t, %v", deleted, err)
	}
	if _, err := repository.Find(ctx, owner.ID, 604); err != nil {
		t.Fatalf("second movie must survive: %v", err)
	}
}

func TestUserMoviesRejectsUnknownUser(t *testing.T) {
	_, pool := newTestDatabase(t)
	ctx := context.Background()

	repository := postgres.NewUserMovieRepository(pool)
	_, err := repository.Save(ctx, models.UserMovie{
		UserID:  uuid.New(),
		MovieID: 1,
		Status:  models.UserMovieWantToWatch,
		Movie:   models.MovieCard{MovieID: 1, Title: "Untitled"},
	})
	if err == nil {
		t.Fatal("Save() for an unknown user must fail on the foreign key")
	}
}
