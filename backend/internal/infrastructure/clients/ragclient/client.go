// Package ragclient connects the backend to the Python RAG service over gRPC.
package ragclient

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	ragv1 "github.com/ivanSaichkin/rag-movie-picker/pkg/ragapi/rag/v1"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/ports"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/platform/requestid"
)

const requestIDMetadataKey = "x-request-id"

var _ ports.RAGClient = (*RAGClient)(nil)

// RAGClient adapts the generated gRPC client to the domain contracts. The
// generated client is injected, so tests run against a bufconn server.
type RAGClient struct {
	client  ragv1.RagServiceClient
	timeout time.Duration
}

// NewRAGClient builds the adapter with a per-call deadline.
func NewRAGClient(client ragv1.RagServiceClient, timeout time.Duration) *RAGClient {
	return &RAGClient{client: client, timeout: timeout}
}

// Catalog fetches the search metadata of the RAG index.
func (c *RAGClient) Catalog(ctx context.Context) (models.Catalog, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	catalog, err := c.client.GetCatalog(c.withRequestID(ctx), &ragv1.GetCatalogRequest{})
	if err != nil {
		return models.Catalog{}, mapError(err)
	}
	return catalogFromProto(catalog), nil
}

// Movie fetches one movie card.
func (c *RAGClient) Movie(ctx context.Context, movieID int64) (models.MovieCard, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	movie, err := c.client.GetMovie(c.withRequestID(ctx), &ragv1.GetMovieRequest{MovieId: movieID})
	if err != nil {
		return models.MovieCard{}, mapError(err)
	}
	return movieFromProto(movie), nil
}

// withRequestID propagates the incoming request identifier to the RAG service,
// so its logs can be correlated with the Go logs.
func (c *RAGClient) withRequestID(ctx context.Context) context.Context {
	id := requestid.FromContext(ctx)
	if id == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, requestIDMetadataKey, id)
}

// mapError converts gRPC status codes into domain errors; anything unexpected
// keeps the original error for the log.
func mapError(err error) error {
	switch status.Code(err) {
	case codes.InvalidArgument:
		return rules.ErrInvalidArgument
	case codes.NotFound:
		return rules.ErrNotFound
	case codes.Unavailable:
		return rules.ErrUnavailable
	case codes.DeadlineExceeded:
		return rules.ErrDeadlineExceeded
	default:
		return fmt.Errorf("rag request failed: %w", err)
	}
}

func catalogFromProto(catalog *ragv1.Catalog) models.Catalog {
	if catalog == nil {
		return models.Catalog{}
	}
	return models.Catalog{
		Genres:              genresFromProto(catalog.GetGenres()),
		MovieCount:          int64(catalog.GetMovieCount()),
		YearFrom:            optionalInt32(catalog.YearFrom),
		YearTo:              optionalInt32(catalog.YearTo),
		DefaultResults:      int(catalog.GetDefaultResults()),
		MaxResults:          int(catalog.GetMaxResults()),
		AvailableStrategies: append([]string(nil), catalog.GetAvailableStrategies()...),
		DefaultStrategy:     catalog.GetDefaultStrategy(),
	}
}

func movieFromProto(movie *ragv1.Movie) models.MovieCard {
	if movie == nil {
		return models.MovieCard{}
	}
	return models.MovieCard{
		MovieID:   movie.GetMovieId(),
		Title:     movie.GetTitle(),
		Year:      optionalInt32(movie.Year),
		Genres:    genresFromProto(movie.GetGenres()),
		Rating:    optionalFloat64(movie.Rating),
		VoteCount: movie.GetVoteCount(),
		Overview:  movie.GetOverview(),
	}
}

func genresFromProto(genres []*ragv1.Genre) []models.Genre {
	result := make([]models.Genre, 0, len(genres))
	for _, genre := range genres {
		if genre == nil {
			continue
		}
		result = append(result, models.Genre{ID: genre.GetId(), Name: genre.GetName()})
	}
	return result
}

func optionalInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func optionalFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
