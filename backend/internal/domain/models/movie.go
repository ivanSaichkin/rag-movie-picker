package models

// MovieCard is a movie from the RAG catalog. Unknown values stay nil, so a
// missing year or rating is not rendered as zero.
type MovieCard struct {
	MovieID   int64
	Title     string
	Year      *int32
	Genres    []Genre
	Rating    *float64
	VoteCount int64
	Overview  string
}
