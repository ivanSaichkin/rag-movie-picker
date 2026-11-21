package rules

import (
	"strings"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
)

// ParseUserMovieStatus reads the status query parameter.
func ParseUserMovieStatus(raw string) (models.UserMovieStatus, error) {
	status := models.UserMovieStatus(strings.TrimSpace(raw))
	if status == "" {
		return "", &ValidationError{Message: "Укажите статус: want_to_watch или watched."}
	}
	if !status.Valid() {
		return "", &ValidationError{Message: "Неизвестный статус фильма."}
	}
	return status, nil
}

// ValidateUserMovieUpdate checks the status and the rating rule: an opinion is
// allowed only for a watched movie.
func ValidateUserMovieUpdate(update models.UserMovieUpdate) error {
	if !update.Status.Valid() {
		return &ValidationError{Message: "Укажите статус: want_to_watch или watched."}
	}
	if update.Status == models.UserMovieWantToWatch && update.Liked != nil {
		return &ValidationError{Message: "Оценка доступна только для просмотренного фильма."}
	}
	return nil
}

// NormalizedUserMovieLiked applies the transition rules: moving a movie back
// to "want to watch" clears the rating, keeping it as "watched" or moving it
// there keeps what the client sent.
func NormalizedUserMovieLiked(update models.UserMovieUpdate) *bool {
	if update.Status == models.UserMovieWantToWatch {
		return nil
	}
	return update.Liked
}
