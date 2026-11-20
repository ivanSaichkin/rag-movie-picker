import csv
import json
import logging
from datetime import date
from pathlib import Path

from pydantic import ValidationError

from movie_rag.models import Genre, Movie


class MovieFormatter:
    version = "movie-text-v1"

    def format(self, movie: Movie) -> str:
        names = list(dict.fromkeys(name for name in [movie.title, movie.original_title] if name))
        return ". ".join([*names, movie.overview, ", ".join(g.name for g in movie.genres), ", ".join(movie.keywords)])


def load_movies(path: Path) -> list[Movie]:
    movies: dict[int, Movie] = {}
    with path.open(encoding="utf-8-sig", newline="") as stream:
        for line, row in enumerate(csv.DictReader(stream), start=2):
            try:
                votes = int(row.get("vote_count") or 0)
                release = row.get("release_date")
                movie = Movie(
                    movie_id=int(row["id"]),
                    title=row["title"].strip(),
                    overview=row["overview"].strip(),
                    original_title=(row.get("original_title") or "").strip(),
                    genres=[Genre.model_validate(g) for g in json.loads(row.get("genres") or "[]")],
                    keywords=[str(k["name"]) for k in json.loads(row.get("keywords") or "[]")],
                    year=date.fromisoformat(release).year if release else None,
                    rating=float(row["vote_average"]) if votes > 0 and row.get("vote_average") else None,
                    vote_count=votes,
                )
            except (ValueError, KeyError, TypeError, ValidationError):
                logging.warning("CSV row skipped: line=%s reason=invalid_movie", line)
                continue
            if movie.movie_id in movies and movies[movie.movie_id] != movie:
                raise ValueError(f"Conflicting duplicate movie ID {movie.movie_id}")
            movies[movie.movie_id] = movie
    if not movies:
        raise ValueError("CSV contains no valid movies")
    return sorted(movies.values(), key=lambda movie: movie.movie_id)
