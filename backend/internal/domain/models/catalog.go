package models

// Genre is a movie genre from the RAG catalog.
type Genre struct {
	ID   int64
	Name string
}

// Catalog is the search metadata reported by the RAG service. Optional bounds
// stay pointers to keep "unknown" distinct from zero.
type Catalog struct {
	Genres              []Genre
	MovieCount          int64
	YearFrom            *int32
	YearTo              *int32
	DefaultResults      int
	MaxResults          int
	AvailableStrategies []string
	DefaultStrategy     string
}
