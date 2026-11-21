package models

import (
	"time"

	"github.com/google/uuid"
)

// UserMovieStatus is the state of a movie in a user's personal list.
type UserMovieStatus string

const (
	// UserMovieWantToWatch marks a movie the user wants to watch.
	UserMovieWantToWatch UserMovieStatus = "want_to_watch"
	// UserMovieWatched marks a movie the user has already watched.
	UserMovieWatched UserMovieStatus = "watched"
)

// Valid reports whether the status is known.
func (s UserMovieStatus) Valid() bool {
	return s == UserMovieWantToWatch || s == UserMovieWatched
}

// UserMovieUpdate is the body of PUT /my/movies/{id}.
type UserMovieUpdate struct {
	Status UserMovieStatus
	Liked  *bool
}

// UserMovie is a movie in the personal list. Movie is the snapshot stored on
// the first write; it is not taken from the browser and never refreshed
// automatically.
type UserMovie struct {
	UserID    uuid.UUID
	MovieID   int64
	Status    UserMovieStatus
	Liked     *bool
	Movie     MovieCard
	UpdatedAt time.Time
}

// UserMoviePage is one page of the personal list.
type UserMoviePage struct {
	Items    []UserMovie
	Page     int
	PageSize int
	Total    int64
}
