import hashlib
import re
import unicodedata
from pathlib import Path

from movie_rag.models import Filters, Movie, ServiceError
from movie_rag.preprocessing import load_movies


def normalize_title(title: str) -> str:
    text = unicodedata.normalize("NFKC", title).casefold().replace("ё", "е")
    return " ".join(re.findall(r"\w+", text))


class CatalogTitleLookup:
    """Exact title lookup over the complete, checksum-verified indexed catalogue."""

    def __init__(self, movies: list[Movie]) -> None:
        self._titles: dict[str, dict[int, Movie]] = {}
        for movie in movies:
            for title in (movie.title, movie.original_title):
                if title.strip():
                    self._titles.setdefault(normalize_title(title), {})[movie.movie_id] = movie

    @classmethod
    def from_csv(cls, path: Path, checksum: str) -> "CatalogTitleLookup":
        if hashlib.sha256(path.read_bytes()).hexdigest() != checksum:
            raise ServiceError("UNAVAILABLE", "Title catalogue differs from indexed CSV; rebuild the index")
        return cls(load_movies(path))

    def find(self, title: str, filters: Filters) -> list[Movie]:
        matches = self._titles.get(normalize_title(title), {}).values()
        return sorted(
            (
                movie
                for movie in matches
                if (not filters.genre_ids or set(filters.genre_ids).intersection(genre.id for genre in movie.genres))
                and (filters.year_from is None or movie.year is not None and movie.year >= filters.year_from)
                and (filters.year_to is None or movie.year is not None and movie.year <= filters.year_to)
                and (filters.min_rating is None or movie.rating is not None and movie.rating >= filters.min_rating)
            ),
            key=lambda movie: movie.movie_id,
        )
