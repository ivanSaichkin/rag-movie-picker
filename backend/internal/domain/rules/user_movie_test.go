package rules_test

import (
	"errors"
	"testing"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
)

func boolPointer(value bool) *bool { return &value }

func TestParseUserMovieStatus(t *testing.T) {
	for _, raw := range []string{"want_to_watch", "watched", " watched "} {
		status, err := rules.ParseUserMovieStatus(raw)
		if err != nil {
			t.Fatalf("ParseUserMovieStatus(%q) error = %v", raw, err)
		}
		if !status.Valid() {
			t.Fatalf("status %q is not valid", status)
		}
	}

	for _, raw := range []string{"", "   ", "maybe"} {
		_, err := rules.ParseUserMovieStatus(raw)
		var validationErr *rules.ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("ParseUserMovieStatus(%q) error = %v, want a validation error", raw, err)
		}
	}
}

func TestValidateUserMovieUpdate(t *testing.T) {
	valid := []models.UserMovieUpdate{
		{Status: models.UserMovieWantToWatch},
		{Status: models.UserMovieWatched},
		{Status: models.UserMovieWatched, Liked: boolPointer(true)},
		{Status: models.UserMovieWatched, Liked: boolPointer(false)},
	}
	for _, update := range valid {
		if err := rules.ValidateUserMovieUpdate(update); err != nil {
			t.Fatalf("ValidateUserMovieUpdate(%+v) error = %v", update, err)
		}
	}

	invalid := []models.UserMovieUpdate{
		{Status: ""},
		{Status: "maybe"},
		{Status: models.UserMovieWantToWatch, Liked: boolPointer(true)},
		{Status: models.UserMovieWantToWatch, Liked: boolPointer(false)},
	}
	for _, update := range invalid {
		err := rules.ValidateUserMovieUpdate(update)
		var validationErr *rules.ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("ValidateUserMovieUpdate(%+v) error = %v, want a validation error", update, err)
		}
	}
}

func TestNormalizedUserMovieLiked(t *testing.T) {
	liked := true

	if got := rules.NormalizedUserMovieLiked(models.UserMovieUpdate{Status: models.UserMovieWantToWatch, Liked: &liked}); got != nil {
		t.Fatalf("want_to_watch liked = %v, want nil", got)
	}

	got := rules.NormalizedUserMovieLiked(models.UserMovieUpdate{Status: models.UserMovieWatched, Liked: &liked})
	if got == nil || !*got {
		t.Fatalf("watched liked = %v, want true", got)
	}

	if got := rules.NormalizedUserMovieLiked(models.UserMovieUpdate{Status: models.UserMovieWatched}); got != nil {
		t.Fatalf("watched without rating liked = %v, want nil", got)
	}
}
