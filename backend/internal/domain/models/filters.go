package models

// Filters narrow the recommendation search. Optional fields stay pointers to
// keep "not set" distinct from zero.
type Filters struct {
	GenreIDs  []int64
	YearFrom  *int32
	YearTo    *int32
	MinRating *float64
}
