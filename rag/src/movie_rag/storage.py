import math

from qdrant_client import QdrantClient
from qdrant_client import models as qm

from movie_rag.models import Budget, Filters, Movie, SearchHit, ServiceError, SparseVector, VectorRepresentation


class QdrantStore:
    def __init__(self, client: QdrantClient, collection: str) -> None:
        self.client, self.collection = client, collection

    def create(self, dimension: int) -> None:
        # Never delete or silently overwrite an existing collection.
        if self.client.collection_exists(self.collection):
            raise ValueError("Collection already exists; select a new collection and state directory for rebuilding")
        self.client.create_collection(
            self.collection,
            vectors_config={"semantic": qm.VectorParams(size=dimension, distance=qm.Distance.COSINE)},
            sparse_vectors_config={"bm25": qm.SparseVectorParams()},
        )
        for name, schema in [
            ("genre_ids", qm.PayloadSchemaType.INTEGER),
            ("year", qm.PayloadSchemaType.INTEGER),
            ("rating", qm.PayloadSchemaType.FLOAT),
            ("index_id", qm.PayloadSchemaType.KEYWORD),
        ]:
            self.client.create_payload_index(self.collection, name, schema)

    def verify(self, dimension: int, count: int, index_id: str) -> None:
        try:
            info = self.client.get_collection(self.collection)
            dense = info.config.params.vectors
            sparse = info.config.params.sparse_vectors or {}
            if not isinstance(dense, dict) or set(dense) != {"semantic"} or "bm25" not in sparse:
                raise ValueError("Missing named vectors")
            if dense["semantic"].size != dimension or dense["semantic"].distance != qm.Distance.COSINE:
                raise ValueError("Dense schema mismatch")
            if sparse["bm25"].modifier is not None:
                raise ValueError("Automatic sparse IDF must be disabled")
            match = qm.Filter(must=[qm.FieldCondition(key="index_id", match=qm.MatchValue(value=index_id))])
            if self.client.count(self.collection, exact=True).count != count:
                raise ValueError("Index count mismatch")
            if self.client.count(self.collection, count_filter=match, exact=True).count != count:
                raise ValueError("Index version mismatch")
        except Exception as exc:
            raise ServiceError("UNAVAILABLE", "Search index is not ready") from exc

    def upsert(self, movies: list[Movie], vectors: list[VectorRepresentation], index_id: str) -> None:
        if len(movies) != len(vectors):
            raise ValueError("Movies and vectors must align")
        points = []
        for movie, vector in zip(movies, vectors, strict=True):
            if vector.dense is None or vector.sparse is None:
                raise ValueError("Both vectors are required at import")
            payload = movie.model_dump()
            payload.update(genre_ids=[g.id for g in movie.genres], index_id=index_id)
            points.append(
                qm.PointStruct(
                    id=movie.movie_id,
                    payload=payload,
                    vector={"semantic": vector.dense, "bm25": qm.SparseVector(**vector.sparse.model_dump())},
                )
            )
        self.client.upsert(self.collection, points=points, wait=True)

    @staticmethod
    def _movie(payload: dict) -> Movie:
        return Movie.model_validate({k: v for k, v in payload.items() if k in Movie.model_fields})

    def get(self, movie_id: int) -> Movie | None:
        try:
            hits = self.client.retrieve(self.collection, ids=[movie_id], with_payload=True)
            return self._movie(hits[0].payload) if hits else None
        except Exception as exc:
            raise ServiceError("UNAVAILABLE", "Movie storage unavailable") from exc

    @staticmethod
    def _filter(filters: Filters) -> qm.Filter | None:
        conditions = []
        if filters.genre_ids:
            conditions.append(qm.FieldCondition(key="genre_ids", match=qm.MatchAny(any=filters.genre_ids)))
        if filters.year_from is not None or filters.year_to is not None:
            conditions.append(qm.FieldCondition(key="year", range=qm.Range(gte=filters.year_from, lte=filters.year_to)))
        if filters.min_rating is not None:
            conditions.append(qm.FieldCondition(key="rating", range=qm.Range(gte=filters.min_rating)))
        return qm.Filter(must=conditions) if conditions else None

    def _search(
        self, vector: list[float] | qm.SparseVector, name: str, filters: Filters, limit: int, budget: Budget
    ) -> list[SearchHit]:
        remaining = budget.remaining()
        try:
            result = self.client.query_points(
                self.collection,
                query=vector,
                using=name,
                query_filter=self._filter(filters),
                limit=limit,
                with_payload=True,
                timeout=max(1, math.ceil(remaining)),
            )
            budget.remaining()
            hits = [SearchHit(self._movie(p.payload), p.score) for p in result.points if name != "bm25" or p.score > 0]
            return sorted(hits, key=lambda hit: (-hit.score, hit.movie.movie_id))
        except ServiceError:
            raise
        except Exception as exc:
            raise ServiceError("UNAVAILABLE", "Vector search unavailable") from exc

    def search_dense(self, vector: list[float], filters: Filters, limit: int, budget: Budget) -> list[SearchHit]:
        return self._search(vector, "semantic", filters, limit, budget)

    def search_sparse(self, vector: SparseVector, filters: Filters, limit: int, budget: Budget) -> list[SearchHit]:
        if not vector.indices:
            return []
        return self._search(qm.SparseVector(**vector.model_dump()), "bm25", filters, limit, budget)
