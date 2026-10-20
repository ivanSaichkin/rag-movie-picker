from typing import Protocol

from movie_rag.models import (
    Budget,
    Filters,
    Movie,
    PreparedQuery,
    RecommendRequest,
    RecommendResponse,
    RetrievalResult,
    SearchHit,
    SparseVector,
    Strategy,
    VectorRepresentation,
)


class EmbeddingModel(Protocol):
    def encode_documents(self, texts: list[str]) -> list[VectorRepresentation]: ...
    def encode_query(self, text: str) -> VectorRepresentation: ...


class DenseEmbeddingModel(EmbeddingModel, Protocol):
    dimension: int
    max_length: int
    truncated_documents: int


class VectorStore(Protocol):
    def upsert(self, movies: list[Movie], vectors: list[VectorRepresentation], index_id: str) -> None: ...
    def get(self, movie_id: int) -> Movie | None: ...
    def search_dense(self, vector: list[float], filters: Filters, limit: int, budget: Budget) -> list[SearchHit]: ...
    def search_sparse(self, vector: SparseVector, filters: Filters, limit: int, budget: Budget) -> list[SearchHit]: ...


class LLMClient(Protocol):
    def complete_json(self, system: str, user: str, budget: Budget) -> dict: ...


class Readiness(Protocol):
    def check(self) -> None: ...


class IndexStore(VectorStore, Protocol):
    def create(self, dimension: int) -> None: ...
    def verify(self, dimension: int, count: int, index_id: str) -> None: ...


class Retriever(Protocol):
    def search(
        self,
        query: PreparedQuery,
        filters: Filters,
        limit: int,
        strategy: Strategy,
        budget: Budget,
    ) -> RetrievalResult: ...


class Recommender(Protocol):
    def recommend(self, request: RecommendRequest, budget: Budget) -> RecommendResponse: ...
