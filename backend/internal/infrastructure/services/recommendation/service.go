// Package recommendation validates search requests, calls RAG and stores the
// result snapshot in the history.
package recommendation

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/ports"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
)

var _ ports.RecommendationService = (*Service)(nil)

// Service implements ports.RecommendationService.
type Service struct {
	catalog ports.CatalogService
	rag     ports.RAGClient
	history ports.HistoryRepository
	logger  *slog.Logger
}

// NewService builds the recommendation service.
func NewService(catalog ports.CatalogService, rag ports.RAGClient, history ports.HistoryRepository, logger *slog.Logger) *Service {
	return &Service{catalog: catalog, rag: rag, history: history, logger: logger}
}

// Recommend validates the request against the catalog, calls RAG and stores the
// snapshot. A failed snapshot is reported as a warning, not as a request error:
// the user still gets the result.
func (s *Service) Recommend(ctx context.Context, userID uuid.UUID, request models.RecommendationRequest) (models.RecommendationResult, error) {
	request.Query = rules.NormalizeQuery(request.Query)
	if err := rules.ValidateQuery(request.Query); err != nil {
		return models.RecommendationResult{}, err
	}

	catalog, err := s.catalog.Catalog(ctx)
	if err != nil {
		return models.RecommendationResult{}, err
	}
	if err := rules.ValidateFilters(request.Filters, catalog); err != nil {
		return models.RecommendationResult{}, err
	}
	if err := rules.ValidateStrategy(request.Strategy, catalog); err != nil {
		return models.RecommendationResult{}, err
	}

	limit, err := rules.ResolveLimit(request.Limit, catalog)
	if err != nil {
		return models.RecommendationResult{}, err
	}
	request.Limit = &limit

	response, err := s.rag.Recommend(ctx, request)
	if err != nil {
		return models.RecommendationResult{}, err
	}

	result := models.RecommendationResult{Response: response}
	entry, err := s.history.Create(ctx, models.HistoryEntry{
		UserID:         userID,
		Query:          request.Query,
		Filters:        request.Filters,
		RequestedLimit: limit,
		Response:       response,
	})
	if err != nil {
		s.logger.Warn("history snapshot was not stored", "error", err)
		result.Response.Warnings = append(result.Response.Warnings, models.Warning{
			Code:    models.WarningHistoryNotSaved,
			Message: "История не сохранена.",
		})
		return result, nil
	}

	result.HistoryID = &entry.ID
	return result, nil
}
