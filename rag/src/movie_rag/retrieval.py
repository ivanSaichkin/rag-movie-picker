from movie_rag.models import Budget, Filters, PreparedQuery, RetrievalResult, SearchHit, ServiceError, Strategy, Warning
from movie_rag.ports import EmbeddingModel, VectorStore


def reciprocal_rank_fusion(branches: list[list[SearchHit]], limit: int, k: int = 60) -> list[SearchHit]:
    if k <= 0:
        raise ValueError("RRF k must be positive")
    scores: dict[int, float] = {}
    movies = {}
    for branch in branches:
        seen = set()
        for rank, hit in enumerate(branch, start=1):
            movie_id = hit.movie.movie_id
            if movie_id in seen:
                continue
            seen.add(movie_id)
            movies[movie_id] = hit.movie
            scores[movie_id] = scores.get(movie_id, 0) + 1 / (k + rank)
    ordered = sorted(scores, key=lambda movie_id: (-scores[movie_id], movie_id))
    return [SearchHit(movies[i], scores[i]) for i in ordered[:limit]]


class RetrievalPipeline:
    def __init__(self, sparse: EmbeddingModel, dense: EmbeddingModel | None, store: VectorStore, rrf_k: int = 60) -> None:
        self.sparse, self.dense, self.store, self.rrf_k = sparse, dense, store, rrf_k

    def search(self, query: PreparedQuery, filters: Filters, limit: int, strategy: Strategy, budget: Budget) -> RetrievalResult:
        results: dict[str, list[SearchHit]] = {}
        errors = []
        branches = ["sparse", "dense"] if strategy == "hybrid" else [strategy]
        for branch in branches:
            budget.remaining()
            try:
                if branch == "sparse":
                    if query.sparse_en is None:
                        raise ServiceError("UNAVAILABLE", "English query unavailable")
                    vector = self.sparse.encode_query(query.sparse_en).sparse
                    if vector is None:
                        raise ValueError("Sparse encoder returned no sparse vector")
                    results[branch] = self.store.search_sparse(vector, filters, limit, budget)
                else:
                    if self.dense is None:
                        raise ServiceError("UNAVAILABLE", "Dense encoder unavailable")
                    dense = self.dense.encode_query(query.original).dense
                    budget.remaining()
                    if dense is None:
                        raise ValueError("Dense encoder returned no dense vector")
                    results[branch] = self.store.search_dense(dense, filters, limit, budget)
            except ServiceError as exc:
                if exc.code in {"CANCELLED", "DEADLINE_EXCEEDED"}:
                    raise
                errors.append(exc)
        if not results:
            raise ServiceError("UNAVAILABLE", "All requested retrieval branches failed") from (errors[0] if errors else None)
        used = "hybrid" if len(results) == 2 else next(iter(results))
        warnings = [Warning(code="RETRIEVAL_DEGRADED", message="Одна из веток поиска недоступна.")] if errors else []
        hits = (
            reciprocal_rank_fusion(list(results.values()), limit, self.rrf_k)
            if len(results) == 2
            else next(iter(results.values()))
        )
        return RetrievalResult(hits[:limit], used, warnings)
