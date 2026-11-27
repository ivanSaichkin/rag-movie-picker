import csv
import hashlib
import json
from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class MovieFilters:
    genre_ids: tuple[int, ...] = ()
    year_from: int | None = None
    year_to: int | None = None
    min_rating: float | None = None

    @classmethod
    def from_dict(cls, values: dict) -> "MovieFilters":
        if set(values) - {"genre_ids", "year_from", "year_to", "min_rating"}:
            raise ValueError("Unknown filter fields")
        genres = values.get("genre_ids", [])
        if not isinstance(genres, list) or any(type(value) is not int or value <= 0 for value in genres):
            raise ValueError("genre_ids must be positive integers")
        result = cls(**{**values, "genre_ids": tuple(genres)})
        for year in (result.year_from, result.year_to):
            if year is not None and (type(year) is not int or not 1 <= year <= 9999):
                raise ValueError("Year must be an integer in 1..9999")
        if result.year_from is not None and result.year_to is not None and result.year_from > result.year_to:
            raise ValueError("Invalid year range")
        if result.min_rating is not None and (type(result.min_rating) not in (float, int) or not 0 <= result.min_rating <= 10):
            raise ValueError("Invalid minimum rating")
        return result


@dataclass(frozen=True)
class Judgment:
    movie_id: int
    title: str
    relevance: int


@dataclass(frozen=True)
class GoldenQuery:
    query_id: str
    query: str
    query_ru: str
    filters: MovieFilters
    judgments: tuple[Judgment, ...]
    kind: str = "normal"
    expected_answer: str = ""


@dataclass(frozen=True)
class CatalogMovie:
    movie_id: int
    title: str
    genre_ids: frozenset[int]
    year: int | None
    rating: float | None


def file_hash(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def load_catalog(path: Path) -> dict[int, CatalogMovie]:
    movies = {}
    with path.open(encoding="utf-8-sig", newline="") as stream:
        for row in csv.DictReader(stream):
            if not row["overview"].strip():
                continue
            movie_id = int(row["id"])
            genres = frozenset(int(genre["id"]) for genre in json.loads(row["genres"] or "[]"))
            rating = float(row["vote_average"]) if int(row["vote_count"] or 0) > 0 and row["vote_average"] else None
            year = int(row["release_date"][:4]) if row["release_date"] else None
            movies[movie_id] = CatalogMovie(movie_id, row["title"], genres, year, rating)
    return movies


def passes_filters(movie: CatalogMovie, filters: MovieFilters) -> bool:
    return (
        (not filters.genre_ids or bool(movie.genre_ids.intersection(filters.genre_ids)))
        and (filters.year_from is None or movie.year is not None and movie.year >= filters.year_from)
        and (filters.year_to is None or movie.year is not None and movie.year <= filters.year_to)
        and (filters.min_rating is None or movie.rating is not None and movie.rating >= filters.min_rating)
    )


def load_goldenset(path: Path, catalog: dict[int, CatalogMovie]) -> list[GoldenQuery]:
    cases = []
    identifiers = set()
    with path.open(encoding="utf-8") as stream:
        for line_number, line in enumerate(stream, start=1):
            if not line.strip():
                continue
            try:
                raw = json.loads(line)
                identifier, query = raw["query_id"], raw["query"]
                if not isinstance(identifier, str) or not identifier or identifier in identifiers:
                    raise ValueError("Invalid or duplicate query_id")
                if not isinstance(query, str) or not query.strip():
                    raise ValueError("query must be a non-empty string")
                filters = MovieFilters.from_dict(raw.get("filters", {}))
                judgments = tuple(_judgment(item, catalog, filters) for item in raw["relevant_movies"])
                kind = raw.get("kind", "normal")
                if kind not in {"normal", "trap"}:
                    raise ValueError("Unknown question kind")
                if len({item.movie_id for item in judgments}) != len(judgments):
                    raise ValueError("Duplicate movie judgments")
                if kind == "normal" and not any(item.relevance >= 2 for item in judgments):
                    raise ValueError("Query needs at least one relevant movie")
                if kind == "trap" and (judgments or not raw.get("expected_answer")):
                    raise ValueError("Trap must have no relevant movies and must specify expected rejection")
                cases.append(
                    GoldenQuery(
                        identifier, query, raw.get("query_ru", ""), filters, judgments, kind, raw.get("expected_answer", "")
                    )
                )
                identifiers.add(identifier)
            except (KeyError, TypeError, ValueError) as error:
                raise ValueError(f"Invalid golden set line {line_number}: {error}") from error
    if not cases:
        raise ValueError("Golden set is empty")
    return cases


def _judgment(raw: dict, catalog: dict[int, CatalogMovie], filters: MovieFilters) -> Judgment:
    judgment = Judgment(**raw)
    if type(judgment.movie_id) is not int or type(judgment.relevance) is not int or not 0 <= judgment.relevance <= 3:
        raise ValueError("Invalid movie ID or relevance grade")
    movie = catalog.get(judgment.movie_id)
    if movie is None or movie.title != judgment.title:
        raise ValueError(f"Movie ID/title not found in dataset: {judgment}")
    if judgment.relevance > 0 and not passes_filters(movie, filters):
        raise ValueError(f"Relevant movie violates query filters: {judgment.title}")
    return judgment
