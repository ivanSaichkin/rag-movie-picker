import pytest
from conftest import Ready, ScriptedLLM

from movie_rag.config import BM25Config, LLMConfig, SearchConfig
from movie_rag.models import Budget, RecommendRequest, RetrievalResult, SearchHit, ServiceError
from movie_rag.service import RagService


class FakePipeline:
    def __init__(self, movies):
        self.movies = movies

    def search(self, query, filters, limit, strategy, budget):
        self.last_query, self.last_limit, self.last_strategy = query, limit, strategy
        return RetrievalResult([SearchHit(movie, 1) for movie in self.movies], strategy)


def test_request_strategy_and_explanation_override(movies, catalog):
    pipeline = FakePipeline(movies)
    llm = ScriptedLLM([])
    service = RagService(pipeline, llm, Ready(), catalog)
    response = service.recommend(
        RecommendRequest(query="Космос", strategy="transformer", generate_explanations=False, limit=1), Budget.seconds(10)
    )
    assert response.mode == "retrieval_only"
    assert response.strategy_used == pipeline.last_strategy == "transformer"
    assert llm.calls == [] and not response.warnings


def test_bm25_without_llm_and_russian_query_rejected(movies, catalog):
    pipeline = FakePipeline(movies)
    llm = ScriptedLLM([])
    service = RagService(
        pipeline,
        llm,
        Ready(),
        catalog,
        SearchConfig(default_strategy="bm25"),
        BM25Config(query_preparation="none"),
        LLMConfig(enabled=False),
    )
    result = service.recommend(RecommendRequest(query="space", limit=1), Budget.seconds(10))
    assert result.strategy_used == "bm25" and result.mode == "retrieval_only"
    assert pipeline.last_query.sparse_en == "space" and not llm.calls
    with pytest.raises(ServiceError, match="English"):
        service.recommend(RecommendRequest(query="космос"), Budget.seconds(10))


def test_translation_is_independent_of_explanations(movies, catalog):
    llm = ScriptedLLM([{"search_query_en": "space"}])
    result = RagService(FakePipeline(movies), llm, Ready(), catalog).recommend(
        RecommendRequest(query="Космос", strategy="bm25", generate_explanations=False, limit=1), Budget.seconds(10)
    )
    assert result.mode == "retrieval_only" and len(llm.calls) == 1


@pytest.mark.parametrize(
    ("query", "expected_strategy", "expected_sparse"),
    [("space travel", "hybrid", "space travel"), ("Космос", "transformer", None)],
)
def test_disable_llm_skips_translation_and_generation(movies, catalog, query, expected_strategy, expected_sparse):
    pipeline = FakePipeline(movies)
    llm = ScriptedLLM([])
    response = RagService(pipeline, llm, Ready(), catalog).recommend(
        RecommendRequest(query=query, disable_llm=True, generate_explanations=True, limit=1), Budget.seconds(10)
    )
    assert llm.calls == []
    assert response.mode == "retrieval_only"
    assert response.strategy_used == pipeline.last_strategy == expected_strategy
    assert pipeline.last_query.sparse_en == expected_sparse


def test_request_can_enable_generation_and_config_controls_candidate_pool(movies, catalog):
    pipeline = FakePipeline(movies)
    large_catalog = catalog.model_copy(update={"movie_count": 100})
    llm = ScriptedLLM([{"items": [{"movie_id": 1, "reason": "Подходит."}]}])
    service = RagService(
        pipeline,
        llm,
        Ready(),
        large_catalog,
        SearchConfig(default_strategy="transformer", generate_explanations=False, min_candidates=5, candidate_multiplier=2),
    )
    result = service.recommend(RecommendRequest(query="space", limit=4, generate_explanations=True), Budget.seconds(10))
    assert result.mode == "generated" and pipeline.last_limit == 8


def test_generated_answer(movies, catalog):
    llm = ScriptedLLM([{"search_query_en": "space"}, {"items": [{"movie_id": 1, "reason": "Космическое приключение."}]}])
    response = RagService(FakePipeline(movies), llm, Ready(), catalog).recommend(
        RecommendRequest(query="Про космос", limit=1),
        Budget.seconds(10),
    )
    assert response.mode == "generated"
    assert response.items[0].movie.movie_id == 1


@pytest.mark.parametrize(
    "selection",
    [
        {"items": [{"movie_id": 999, "reason": "bad"}]},
        {"items": [{"movie_id": 1, "reason": "one"}, {"movie_id": 1, "reason": "duplicate"}]},
        {"items": [{"movie_id": 1, "reason": ""}]},
        {"unexpected": True},
        ServiceError("UNAVAILABLE", "provider failure"),
    ],
)
def test_generation_fallback(selection, movies, catalog):
    llm = ScriptedLLM([{"search_query_en": "space"}, selection])
    result = RagService(FakePipeline(movies), llm, Ready(), catalog).recommend(
        RecommendRequest(query="space", limit=1),
        Budget.seconds(10),
    )
    assert result.mode == "retrieval_only"
    assert len(result.items) == 1 and result.items[0].reason is None
    assert "GENERATION_UNAVAILABLE" in [warning.code for warning in result.warnings]


def test_empty_retrieval_skips_generation(catalog):
    llm = ScriptedLLM([{"search_query_en": "space"}])
    response = RagService(FakePipeline([]), llm, Ready(), catalog).recommend(RecommendRequest(query="space"), Budget.seconds(10))
    assert response.mode == "empty" and len(llm.calls) == 1


def test_dense_skips_translation(movies, catalog):
    llm = ScriptedLLM([{"items": []}])
    response = RagService(FakePipeline(movies), llm, Ready(), catalog, SearchConfig(default_strategy="transformer")).recommend(
        RecommendRequest(query="space"),
        Budget.seconds(10),
    )
    assert response.mode == "empty" and len(llm.calls) == 1


def test_invalid_limit_and_genre(movies, catalog):
    service = RagService(FakePipeline(movies), ScriptedLLM([]), Ready(), catalog)
    for request in [RecommendRequest(query="x", limit=51), RecommendRequest(query="x", filters={"genre_ids": [999]})]:
        with pytest.raises(ServiceError):
            service.recommend(request, Budget.seconds(10))


def test_context_budget_and_deadline(movies, catalog):
    llm = ScriptedLLM([])
    generation = LLMConfig.model_construct(context_bytes=1)
    result = RagService(
        FakePipeline(movies), llm, Ready(), catalog, SearchConfig(default_strategy="transformer"), generation=generation
    ).recommend(
        RecommendRequest(query="space"),
        Budget.seconds(10),
    )
    assert result.mode == "retrieval_only" and not llm.calls
    assert "CONTEXT_LIMIT" in [warning.code for warning in result.warnings]
    with pytest.raises(ServiceError):
        RagService(FakePipeline(movies), llm, Ready(), catalog).recommend(RecommendRequest(query="space"), Budget.seconds(-1))
