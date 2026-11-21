// Package history serves stored search history snapshots.
package history

import (
	"context"

	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/ports"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
)

var _ ports.HistoryService = (*Service)(nil)

// Service implements ports.HistoryService.
type Service struct {
	repository ports.HistoryRepository
}

// NewService builds the history service.
func NewService(repository ports.HistoryRepository) *Service {
	return &Service{repository: repository}
}

// List returns one page of the user's history.
func (s *Service) List(ctx context.Context, userID uuid.UUID, page, pageSize int) (models.HistoryPage, error) {
	offset := (page - 1) * pageSize
	items, total, err := s.repository.List(ctx, userID, pageSize, offset)
	if err != nil {
		return models.HistoryPage{}, err
	}
	return models.HistoryPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

// Get returns one stored snapshot; another user's record is not found.
func (s *Service) Get(ctx context.Context, userID, historyID uuid.UUID) (models.HistoryEntry, error) {
	return s.repository.Get(ctx, userID, historyID)
}

// Delete removes one stored snapshot; another user's record is not found.
func (s *Service) Delete(ctx context.Context, userID, historyID uuid.UUID) error {
	deleted, err := s.repository.Delete(ctx, userID, historyID)
	if err != nil {
		return err
	}
	if !deleted {
		return rules.ErrNotFound
	}
	return nil
}
