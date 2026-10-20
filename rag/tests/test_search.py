import pytest
from conftest import BrokenEncoder, FakeDense
from qdrant_client import QdrantClient

from movie_rag.embeddings import BM25Encoder, HybridEmbedder
from movie_rag.models import Budget, Filters, PreparedQuery, SearchHit, ServiceError
from movie_rag.preprocessing import MovieFormatter
from movie_rag.retrieval import RetrievalPipeline, reciprocal_rank_fusion
from movie_rag.storage import QdrantStore


@pytest.fixture
def search(movies):
    client = QdrantClient(":memory:")
    store = QdrantStore(client, "test")
    store.create(3)
    texts = [MovieFormatter().format(movie) for movie in movies]
    sparse = BM25Encoder()
    sparse.fit(texts)
    store.upsert(movies, HybridEmbedder(sparse, FakeDense()).encode_documents(texts), "test-index")
    yield store, sparse
    client.close()


@pytest.mark.integration
def test_qdrant_sparse_equals_manual(search, movies):
    store, sparse = search
    query = sparse.encode_query("space").sparse
    result = store.search_sparse(query, Filters(), 10, Budget.seconds(10))
    for hit in result:
        vector = sparse.encode_documents([MovieFormatter().format(hit.movie)])[0].sparse
        coordinates = dict(zip(vector.indices, vector.values, strict=True))
        expected = sum(coordinates.get(i, 0) * v for i, v in zip(query.indices, query.values, strict=True))
        assert hit.score == pytest.approx(expected, rel=1e-5)
    assert {hit.movie.movie_id for hit in result} == {1, 3}


@pytest.mark.integration
def test_both_branches_enforce_filters(search):
    store, sparse = search
    pipeline = RetrievalPipeline(sparse, FakeDense(), store)
    result = pipeline.search(
        PreparedQuery("space", "space"), Filters(year_from=2010, min_rating=7, genre_ids=[878]), 10, "hybrid", Budget.seconds(10)
    )
    assert [hit.movie.movie_id for hit in result.hits] == [1]
    assert result.strategy_used == "hybrid"
    store.verify(3, 3, "test-index")
    with pytest.raises(ServiceError):
        store.verify(3, 4, "test-index")


def test_rrf_scores_and_deduplication(movies):
    hits = reciprocal_rank_fusion(
        [[SearchHit(movies[0], 100), SearchHit(movies[1], 1)], [SearchHit(movies[1], 0.9), SearchHit(movies[2], 0.8)]],
        3,
    )
    assert [hit.movie.movie_id for hit in hits] == [2, 1, 3]
    assert hits[0].score == pytest.approx(1 / 61 + 1 / 62)


def test_branch_failure_and_empty_vector(search):
    store, sparse = search
    pipeline = RetrievalPipeline(sparse, BrokenEncoder(), store)
    result = pipeline.search(PreparedQuery("space", "space"), Filters(), 10, "hybrid", Budget.seconds(10))
    assert result.strategy_used == "sparse"
    assert result.warnings[0].code == "RETRIEVAL_DEGRADED"
    pipeline = RetrievalPipeline(sparse, FakeDense(), store)
    assert pipeline.search(PreparedQuery("space", "unknown"), Filters(), 10, "hybrid", Budget.seconds(10)).hits


def test_translation_unavailable_falls_back_to_dense(search):
    store, sparse = search
    result = RetrievalPipeline(sparse, FakeDense(), store).search(
        PreparedQuery("space", None),
        Filters(),
        10,
        "hybrid",
        Budget.seconds(10),
    )
    assert result.strategy_used == "dense"
    assert result.hits
    assert result.warnings[0].code == "RETRIEVAL_DEGRADED"


def test_both_branches_failed_is_not_empty(search):
    store, _ = search
    with pytest.raises(ServiceError, match="All requested"):
        RetrievalPipeline(BrokenEncoder(), BrokenEncoder(), store).search(
            PreparedQuery("space", "space"),
            Filters(),
            10,
            "hybrid",
            Budget.seconds(10),
        )


def test_budget_cancellation(search):
    store, sparse = search
    with pytest.raises(ServiceError, match="cancelled"):
        RetrievalPipeline(sparse, FakeDense(), store).search(
            PreparedQuery("space", "space"),
            Filters(),
            10,
            "hybrid",
            Budget.seconds(10, lambda: False),
        )
