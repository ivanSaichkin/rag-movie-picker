package rules_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
)

func catalogFixture() models.Catalog {
	return models.Catalog{
		Genres:              []models.Genre{{ID: 878, Name: "Science Fiction"}, {ID: 12, Name: "Adventure"}},
		MovieCount:          4803,
		DefaultResults:      10,
		MaxResults:          50,
		AvailableStrategies: []string{"bm25", "transformer", "hybrid"},
		DefaultStrategy:     "hybrid",
	}
}

func intPointer(value int) *int { return &value }

func int32Pointer(value int32) *int32 { return &value }

func float64Pointer(value float64) *float64 { return &value }

func stringPointer(value string) *string { return &value }

func TestNormalizeQuery(t *testing.T) {
	if got := rules.NormalizeQuery("  космос  "); got != "космос" {
		t.Fatalf("NormalizeQuery() = %q", got)
	}
}

func TestValidateQuery(t *testing.T) {
	if err := rules.ValidateQuery(""); err == nil {
		t.Fatal("empty query must be rejected")
	}
	if err := rules.ValidateQuery(strings.Repeat("a", rules.MaxQueryLength+1)); err == nil {
		t.Fatal("too long query must be rejected")
	}
	if err := rules.ValidateQuery(strings.Repeat("я", rules.MaxQueryLength)); err != nil {
		t.Fatalf("MaxQueryLength runes must be accepted: %v", err)
	}
}

func TestResolveLimit(t *testing.T) {
	catalog := catalogFixture()

	resolved, err := rules.ResolveLimit(nil, catalog)
	if err != nil || resolved != 10 {
		t.Fatalf("ResolveLimit(nil) = %d, %v", resolved, err)
	}

	resolved, err = rules.ResolveLimit(intPointer(25), catalog)
	if err != nil || resolved != 25 {
		t.Fatalf("ResolveLimit(25) = %d, %v", resolved, err)
	}

	for _, invalid := range []int{0, -1, 51} {
		if _, err := rules.ResolveLimit(intPointer(invalid), catalog); err == nil {
			t.Fatalf("limit %d must be rejected", invalid)
		}
	}

	withoutDefault := models.Catalog{MaxResults: 50}
	resolved, err = rules.ResolveLimit(nil, withoutDefault)
	if err != nil || resolved != rules.DefaultResultLimit {
		t.Fatalf("fallback limit = %d, %v", resolved, err)
	}

	withoutMaximum := models.Catalog{DefaultResults: 10}
	if _, err := rules.ResolveLimit(intPointer(200), withoutMaximum); err == nil {
		t.Fatal("limit above the fallback maximum must be rejected")
	}
}

func TestValidateFilters(t *testing.T) {
	catalog := catalogFixture()

	valid := models.Filters{
		GenreIDs:  []int64{878, 12},
		YearFrom:  int32Pointer(2000),
		YearTo:    int32Pointer(2015),
		MinRating: float64Pointer(6.5),
	}
	if err := rules.ValidateFilters(valid, catalog); err != nil {
		t.Fatalf("ValidateFilters() error = %v", err)
	}

	invalid := map[string]models.Filters{
		"rating below zero":   {MinRating: float64Pointer(-0.1)},
		"rating above ten":    {MinRating: float64Pointer(10.1)},
		"year before catalog": {YearFrom: int32Pointer(1700)},
		"year after catalog":  {YearTo: int32Pointer(3001)},
		"reversed year range": {YearFrom: int32Pointer(2015), YearTo: int32Pointer(2000)},
		"unknown genre":       {GenreIDs: []int64{999}},
		"one unknown genre":   {GenreIDs: []int64{878, 999}},
	}
	for name, filters := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := rules.ValidateFilters(filters, catalog); err == nil {
				t.Fatalf("filters %+v must be rejected", filters)
			}
			var validationErr *rules.ValidationError
			if err := rules.ValidateFilters(filters, catalog); !errors.As(err, &validationErr) {
				t.Fatalf("error %v must be a validation error", err)
			}
		})
	}

	// An empty catalog is a dependency that did not report genres yet; RAG
	// still validates the request.
	if err := rules.ValidateFilters(models.Filters{GenreIDs: []int64{999}}, models.Catalog{}); err != nil {
		t.Fatalf("empty catalog must not reject genres: %v", err)
	}
}

func TestValidateStrategy(t *testing.T) {
	catalog := catalogFixture()

	if err := rules.ValidateStrategy(nil, catalog); err != nil {
		t.Fatalf("nil strategy must be allowed: %v", err)
	}
	if err := rules.ValidateStrategy(stringPointer(""), catalog); err != nil {
		t.Fatalf("empty strategy must be allowed: %v", err)
	}
	if err := rules.ValidateStrategy(stringPointer("hybrid"), catalog); err != nil {
		t.Fatalf("known strategy must be allowed: %v", err)
	}
	if err := rules.ValidateStrategy(stringPointer("magic"), catalog); err == nil {
		t.Fatal("unknown strategy must be rejected")
	}
	if err := rules.ValidateStrategy(stringPointer("magic"), models.Catalog{}); err != nil {
		t.Fatalf("empty catalog must not validate strategies: %v", err)
	}
}

func TestNormalizePagination(t *testing.T) {
	page, pageSize, err := rules.NormalizePagination(nil, nil)
	if err != nil || page != 1 || pageSize != 20 {
		t.Fatalf("defaults = %d, %d, %v", page, pageSize, err)
	}

	page, pageSize, err = rules.NormalizePagination(intPointer(2), intPointer(50))
	if err != nil || page != 2 || pageSize != 50 {
		t.Fatalf("explicit = %d, %d, %v", page, pageSize, err)
	}

	for name, params := range map[string][2]*int{
		"zero page":         {intPointer(0), nil},
		"negative page":     {intPointer(-1), nil},
		"zero page size":    {nil, intPointer(0)},
		"page size too big": {nil, intPointer(101)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := rules.NormalizePagination(params[0], params[1]); err == nil {
				t.Fatal("invalid pagination must be rejected")
			}
		})
	}
}
