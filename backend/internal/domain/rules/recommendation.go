package rules

import (
	"fmt"
	"strings"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
)

const (
	// MaxQueryLength limits the user query after trimming.
	MaxQueryLength = 1000

	// MinRating is the lower bound of the rating filter.
	MinRating = 0.0
	// MaxRating is the upper bound of the rating filter.
	MaxRating = 10.0
	// MinYear is the lower bound of the year filters.
	MinYear = 1800
	// MaxYear is the upper bound of the year filters.
	MaxYear = 3000

	// DefaultPage applies when the client omits the page.
	DefaultPage = 1
	// DefaultPageSize applies when the client omits the page size.
	DefaultPageSize = 20
	// MaxPageSize limits one page of history.
	MaxPageSize = 100

	// DefaultResultLimit is used when the RAG catalog does not report its
	// default, so that a stub catalog cannot disable recommendations.
	DefaultResultLimit = 10
	// FallbackMaxResults bounds the limit when the catalog reports no maximum.
	FallbackMaxResults = 50
)

// NormalizeQuery trims the user query.
func NormalizeQuery(raw string) string {
	return strings.TrimSpace(raw)
}

// ValidateQuery checks the query length after trimming.
func ValidateQuery(query string) error {
	length := len([]rune(query))
	switch {
	case length == 0:
		return &ValidationError{Message: "Укажите запрос."}
	case length > MaxQueryLength:
		return &ValidationError{Message: fmt.Sprintf("Запрос не должен превышать %d символов.", MaxQueryLength)}
	default:
		return nil
	}
}

// ResolveLimit takes the requested limit or the catalog default and checks it
// against the catalog maximum.
func ResolveLimit(limit *int, catalog models.Catalog) (int, error) {
	maxResult := catalog.MaxResults
	if maxResult <= 0 {
		maxResult = FallbackMaxResults
	}

	if limit == nil {
		if catalog.DefaultResults > 0 && catalog.DefaultResults <= maxResult {
			return catalog.DefaultResults, nil
		}
		return DefaultResultLimit, nil
	}

	if *limit < 1 || *limit > maxResult {
		return 0, &ValidationError{
			Message: fmt.Sprintf("Количество результатов должно быть от 1 до %d.", maxResult),
		}
	}
	return *limit, nil
}

// ValidateFilters checks the filters and that the genres exist in the catalog.
// An empty genre list means the catalog did not report genres; the check is
// skipped then, because RAG still validates the request.
func ValidateFilters(filters models.Filters, catalog models.Catalog) error {
	if filters.MinRating != nil && (*filters.MinRating < MinRating || *filters.MinRating > MaxRating) {
		return &ValidationError{Message: "Рейтинг должен быть в диапазоне от 0 до 10."}
	}
	if filters.YearFrom != nil && (*filters.YearFrom < MinYear || *filters.YearFrom > MaxYear) {
		return &ValidationError{Message: "Некорректный начальный год."}
	}
	if filters.YearTo != nil && (*filters.YearTo < MinYear || *filters.YearTo > MaxYear) {
		return &ValidationError{Message: "Некорректный конечный год."}
	}
	if filters.YearFrom != nil && filters.YearTo != nil && *filters.YearFrom > *filters.YearTo {
		return &ValidationError{Message: "Начальный год не должен быть больше конечного."}
	}

	if len(catalog.Genres) == 0 {
		return nil
	}
	known := make(map[int64]struct{}, len(catalog.Genres))
	for _, genre := range catalog.Genres {
		known[genre.ID] = struct{}{}
	}
	for _, genreID := range filters.GenreIDs {
		if _, ok := known[genreID]; !ok {
			return &ValidationError{Message: "Неизвестный жанр."}
		}
	}
	return nil
}

// ValidateStrategy checks the optional strategy override against the catalog.
func ValidateStrategy(strategy *string, catalog models.Catalog) error {
	if strategy == nil || strings.TrimSpace(*strategy) == "" {
		return nil
	}
	if len(catalog.AvailableStrategies) == 0 {
		return nil
	}
	for _, available := range catalog.AvailableStrategies {
		if available == *strategy {
			return nil
		}
	}
	return &ValidationError{Message: fmt.Sprintf("Неизвестная стратегия поиска %q.", *strategy)}
}

// NormalizePagination resolves the optional page parameters and checks them.
func NormalizePagination(page, pageSize *int) (int, int, error) {
	resolvedPage := DefaultPage
	if page != nil {
		resolvedPage = *page
	}
	resolvedPageSize := DefaultPageSize
	if pageSize != nil {
		resolvedPageSize = *pageSize
	}

	if resolvedPage < 1 {
		return 0, 0, &ValidationError{Message: "Страница должна быть положительным числом."}
	}
	if resolvedPageSize < 1 || resolvedPageSize > MaxPageSize {
		return 0, 0, &ValidationError{
			Message: fmt.Sprintf("Размер страницы должен быть от 1 до %d.", MaxPageSize),
		}
	}
	return resolvedPage, resolvedPageSize, nil
}
