// Package usermovies manages the personal lists of a user.
package usermovies

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/ports"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
)

var _ ports.UserMovieService = (*Service)(nil)

// Service implements ports.UserMovieService. The movie card is taken from RAG
// on the first write and stored as a snapshot; later status changes never
// refresh it.
type Service struct {
	repository ports.UserMovieRepository
	rag        ports.RAGClient
}

// NewService builds the personal list service.
func NewService(repository ports.UserMovieRepository, rag ports.RAGClient) *Service {
	return &Service{repository: repository, rag: rag}
}

// Set creates or updates the list entry. An unknown movie is looked up in RAG,
// so the snapshot comes from the catalog rather than from the browser.
func (s *Service) Set(ctx context.Context, userID uuid.UUID, movieID int64, update models.UserMovieUpdate) (models.UserMovie, error) {
	if err := rules.ValidateUserMovieUpdate(update); err != nil {
		return models.UserMovie{}, err
	}
	liked := rules.NormalizedUserMovieLiked(update)

	existing, err := s.repository.Find(ctx, userID, movieID)
	switch {
	case err == nil:
		existing.Status = update.Status
		existing.Liked = liked
		return s.repository.Save(ctx, existing)
	case errors.Is(err, rules.ErrNotFound):
		card, err := s.rag.Movie(ctx, movieID)
		if err != nil {
			return models.UserMovie{}, err
		}
		return s.repository.Save(ctx, models.UserMovie{
			UserID:  userID,
			MovieID: movieID,
			Status:  update.Status,
			Liked:   liked,
			Movie:   card,
		})
	default:
		return models.UserMovie{}, err
	}
}

// Delete removes the entry; a missing entry is reported as not found.
func (s *Service) Delete(ctx context.Context, userID uuid.UUID, movieID int64) error {
	deleted, err := s.repository.Delete(ctx, userID, movieID)
	if err != nil {
		return err
	}
	if !deleted {
		return rules.ErrNotFound
	}
	return nil
}

// List returns one page of the user's list for the status.
func (s *Service) List(ctx context.Context, userID uuid.UUID, status models.UserMovieStatus, page, pageSize int) (models.UserMoviePage, error) {
	offset := (page - 1) * pageSize
	items, total, err := s.repository.List(ctx, userID, status, pageSize, offset)
	if err != nil {
		return models.UserMoviePage{}, err
	}
	return models.UserMoviePage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}
