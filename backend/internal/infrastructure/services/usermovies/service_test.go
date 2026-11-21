package usermovies_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/services/usermovies"
)

var testUserID = uuid.MustParse("11111111-1111-1111-1111-111111111111")

type fakeRepository struct {
	found    models.UserMovie
	findErr  error
	findCall int

	saved     models.UserMovie
	saveErr   error
	saveCalls int

	deleted     bool
	deleteErr   error
	deleteCalls int

	items   []models.UserMovie
	total   int64
	listErr error
	limit   int
	offset  int
	status  models.UserMovieStatus
}

func (f *fakeRepository) Save(_ context.Context, movie models.UserMovie) (models.UserMovie, error) {
	f.saveCalls++
	f.saved = movie
	if f.saveErr != nil {
		return models.UserMovie{}, f.saveErr
	}
	movie.UpdatedAt = time.Now()
	return movie, nil
}

func (f *fakeRepository) Find(context.Context, uuid.UUID, int64) (models.UserMovie, error) {
	f.findCall++
	return f.found, f.findErr
}

func (f *fakeRepository) Delete(context.Context, uuid.UUID, int64) (bool, error) {
	f.deleteCalls++
	return f.deleted, f.deleteErr
}

func (f *fakeRepository) List(_ context.Context, _ uuid.UUID, status models.UserMovieStatus, limit, offset int) ([]models.UserMovie, int64, error) {
	f.status = status
	f.limit = limit
	f.offset = offset
	return f.items, f.total, f.listErr
}

type fakeRAG struct {
	movie   models.MovieCard
	err     error
	calls   int
	movieID int64
}

func (f *fakeRAG) Movie(_ context.Context, movieID int64) (models.MovieCard, error) {
	f.calls++
	f.movieID = movieID
	return f.movie, f.err
}

func (f *fakeRAG) Catalog(context.Context) (models.Catalog, error) {
	return models.Catalog{}, errors.New("unexpected call")
}

func (f *fakeRAG) Recommend(context.Context, models.RecommendationRequest) (models.Recommendation, error) {
	return models.Recommendation{}, errors.New("unexpected call")
}

func boolPointer(value bool) *bool { return &value }

func sampleMovie() models.MovieCard {
	year := int32(1999)
	rating := 8.2
	return models.MovieCard{
		MovieID:   603,
		Title:     "The Matrix",
		Year:      &year,
		Genres:    []models.Genre{{ID: 28, Name: "Action"}},
		Rating:    &rating,
		VoteCount: 24589,
		Overview:  "A hacker discovers the truth.",
	}
}

func TestSetFirstWriteTakesSnapshotFromRAG(t *testing.T) {
	repository := &fakeRepository{findErr: rules.ErrNotFound}
	rag := &fakeRAG{movie: sampleMovie()}
	service := usermovies.NewService(repository, rag)

	movie, err := service.Set(context.Background(), testUserID, 603, models.UserMovieUpdate{Status: models.UserMovieWantToWatch})
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if rag.calls != 1 || rag.movieID != 603 {
		t.Fatalf("RAG calls = %d, movie id = %d", rag.calls, rag.movieID)
	}
	if repository.saveCalls != 1 {
		t.Fatalf("Save calls = %d", repository.saveCalls)
	}
	if stored := repository.saved; stored.Movie.Title != "The Matrix" || stored.Status != models.UserMovieWantToWatch || stored.Liked != nil {
		t.Fatalf("stored movie = %+v", stored)
	}
	if movie.Movie.Title != "The Matrix" || movie.UpdatedAt.IsZero() {
		t.Fatalf("returned movie = %+v", movie)
	}
}

func TestSetUpdateDoesNotCallRAG(t *testing.T) {
	existing := models.UserMovie{
		UserID:  testUserID,
		MovieID: 603,
		Status:  models.UserMovieWatched,
		Liked:   boolPointer(true),
		Movie:   sampleMovie(),
	}
	repository := &fakeRepository{found: existing}
	rag := &fakeRAG{}
	service := usermovies.NewService(repository, rag)

	movie, err := service.Set(context.Background(), testUserID, 603, models.UserMovieUpdate{Status: models.UserMovieWantToWatch})
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if rag.calls != 0 {
		t.Fatalf("RAG must not be called on update, calls = %d", rag.calls)
	}
	if movie.Status != models.UserMovieWantToWatch || movie.Liked != nil {
		t.Fatalf("movie = %+v, moving back must clear the rating", movie)
	}
	if repository.saved.Movie.Title != "The Matrix" {
		t.Fatalf("snapshot changed: %+v", repository.saved.Movie)
	}
}

func TestSetMovingToWatchedKeepsRating(t *testing.T) {
	repository := &fakeRepository{found: models.UserMovie{
		UserID:  testUserID,
		MovieID: 603,
		Status:  models.UserMovieWantToWatch,
		Movie:   sampleMovie(),
	}}
	service := usermovies.NewService(repository, &fakeRAG{})

	movie, err := service.Set(context.Background(), testUserID, 603, models.UserMovieUpdate{
		Status: models.UserMovieWatched,
		Liked:  boolPointer(true),
	})
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if movie.Liked == nil || !*movie.Liked {
		t.Fatalf("liked = %v, want true", movie.Liked)
	}
}

func TestSetValidatesInput(t *testing.T) {
	cases := map[string]models.UserMovieUpdate{
		"empty status":       {},
		"unknown status":     {Status: "maybe"},
		"liked on want list": {Status: models.UserMovieWantToWatch, Liked: boolPointer(true)},
	}
	for name, update := range cases {
		t.Run(name, func(t *testing.T) {
			repository := &fakeRepository{}
			rag := &fakeRAG{}
			service := usermovies.NewService(repository, rag)

			_, err := service.Set(context.Background(), testUserID, 603, update)
			var validationErr *rules.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %v, want a validation error", err)
			}
			if repository.findCall != 0 || repository.saveCalls != 0 || rag.calls != 0 {
				t.Fatal("invalid update must not touch the repository or RAG")
			}
		})
	}
}

func TestSetPropagatesFindFailure(t *testing.T) {
	findErr := errors.New("database is down")
	repository := &fakeRepository{findErr: findErr}
	service := usermovies.NewService(repository, &fakeRAG{})

	_, err := service.Set(context.Background(), testUserID, 603, models.UserMovieUpdate{Status: models.UserMovieWantToWatch})
	if !errors.Is(err, findErr) {
		t.Fatalf("error = %v, want the repository error", err)
	}
}

func TestSetPropagatesRAGFailureOnFirstWrite(t *testing.T) {
	ragErr := errors.New("rag is down")
	repository := &fakeRepository{findErr: rules.ErrNotFound}
	service := usermovies.NewService(repository, &fakeRAG{err: ragErr})

	_, err := service.Set(context.Background(), testUserID, 603, models.UserMovieUpdate{Status: models.UserMovieWantToWatch})
	if !errors.Is(err, ragErr) {
		t.Fatalf("error = %v, want the RAG error", err)
	}
	if repository.saveCalls != 0 {
		t.Fatal("failed lookup must not be stored")
	}
}

func TestSetPropagatesSaveFailure(t *testing.T) {
	saveErr := errors.New("database is down")
	repository := &fakeRepository{findErr: rules.ErrNotFound, saveErr: saveErr}
	service := usermovies.NewService(repository, &fakeRAG{movie: sampleMovie()})

	_, err := service.Set(context.Background(), testUserID, 603, models.UserMovieUpdate{Status: models.UserMovieWantToWatch})
	if !errors.Is(err, saveErr) {
		t.Fatalf("error = %v, want the repository error", err)
	}
}

func TestDeleteReportsMissingRecord(t *testing.T) {
	repository := &fakeRepository{deleted: false}
	service := usermovies.NewService(repository, &fakeRAG{})

	err := service.Delete(context.Background(), testUserID, 603)
	if !errors.Is(err, rules.ErrNotFound) {
		t.Fatalf("Delete() error = %v, want ErrNotFound", err)
	}

	repository.deleted = true
	if err := service.Delete(context.Background(), testUserID, 603); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

func TestListComputesOffset(t *testing.T) {
	repository := &fakeRepository{
		items: []models.UserMovie{{MovieID: 603, Status: models.UserMovieWatched}},
		total: 21,
	}
	service := usermovies.NewService(repository, &fakeRAG{})

	page, err := service.List(context.Background(), testUserID, models.UserMovieWatched, 3, 10)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if repository.status != models.UserMovieWatched || repository.limit != 10 || repository.offset != 20 {
		t.Fatalf("repository received status=%q limit=%d offset=%d", repository.status, repository.limit, repository.offset)
	}
	if page.Page != 3 || page.PageSize != 10 || page.Total != 21 || len(page.Items) != 1 {
		t.Fatalf("page = %+v", page)
	}
}
