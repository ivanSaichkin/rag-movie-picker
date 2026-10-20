import math
import time
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Literal

from pydantic import BaseModel, ConfigDict, Field, model_validator

Strategy = Literal["sparse", "dense", "hybrid"]


class ServiceError(Exception):
    def __init__(self, code: str, message: str) -> None:
        super().__init__(message)
        self.code = code


@dataclass
class Budget:
    expires: float
    active: Callable[[], bool] = lambda: True

    @classmethod
    def seconds(cls, seconds: float, active: Callable[[], bool] = lambda: True) -> "Budget":
        return cls(time.monotonic() + seconds, active)

    def remaining(self) -> float:
        if not self.active():
            raise ServiceError("CANCELLED", "Request cancelled")
        left = self.expires - time.monotonic()
        if left <= 0:
            raise ServiceError("DEADLINE_EXCEEDED", "Request deadline exceeded")
        return left


class Model(BaseModel):
    model_config = ConfigDict(extra="forbid", allow_inf_nan=False)


class Genre(Model):
    id: int = Field(gt=0)
    name: str = Field(min_length=1)


class Movie(Model):
    movie_id: int = Field(gt=0)
    title: str = Field(min_length=1)
    original_title: str = ""
    overview: str = Field(min_length=1)
    genres: list[Genre] = Field(default_factory=list)
    keywords: list[str] = Field(default_factory=list)
    year: int | None = Field(default=None, ge=1, le=9999)
    rating: float | None = Field(default=None, ge=0, le=10)
    vote_count: int = Field(default=0, ge=0)


class Filters(Model):
    genre_ids: list[int] = Field(default_factory=list)
    year_from: int | None = Field(default=None, ge=1, le=9999)
    year_to: int | None = Field(default=None, ge=1, le=9999)
    min_rating: float | None = Field(default=None, ge=0, le=10)

    @model_validator(mode="after")
    def validate_range(self) -> "Filters":
        if self.year_from is not None and self.year_to is not None and self.year_from > self.year_to:
            raise ValueError("year_from must not exceed year_to")
        if any(genre <= 0 for genre in self.genre_ids):
            raise ValueError("genre IDs must be positive")
        return self


class RecommendRequest(Model):
    query: str = Field(min_length=1, max_length=1000)
    limit: int = Field(default=10, ge=1)
    filters: Filters = Field(default_factory=Filters)


class SparseVector(Model):
    indices: list[int]
    values: list[float]

    @model_validator(mode="after")
    def validate_coordinates(self) -> "SparseVector":
        if len(self.indices) != len(self.values) or len(set(self.indices)) != len(self.indices):
            raise ValueError("Sparse coordinates must be unique and aligned")
        if any(i < 0 for i in self.indices) or any(v < 0 for v in self.values):
            raise ValueError("Sparse coordinates and BM25 weights must be non-negative")
        return self


class VectorRepresentation(Model):
    dense: list[float] | None = None
    sparse: SparseVector | None = None

    @model_validator(mode="after")
    def validate_vector(self) -> "VectorRepresentation":
        if self.dense is None and self.sparse is None:
            raise ValueError("At least one representation is required")
        if self.dense is not None and (not self.dense or not all(math.isfinite(x) for x in self.dense)):
            raise ValueError("Invalid dense vector")
        return self


class Warning(Model):
    code: str
    message: str


class Recommendation(Model):
    movie: Movie
    reason: str | None = None


class RecommendResponse(Model):
    mode: Literal["generated", "retrieval_only", "empty"]
    strategy_used: Strategy
    items: list[Recommendation] = Field(default_factory=list)
    warnings: list[Warning] = Field(default_factory=list)


class Catalog(Model):
    genres: list[Genre]
    movie_count: int
    year_from: int | None = None
    year_to: int | None = None
    default_results: int = 10
    max_results: int = 50


@dataclass
class SearchHit:
    movie: Movie
    score: float


@dataclass
class PreparedQuery:
    original: str
    sparse_en: str | None


@dataclass
class RetrievalResult:
    hits: list[SearchHit]
    strategy_used: Strategy
    warnings: list[Warning] = field(default_factory=list)
