import pytest
from conftest import Ready, ScriptedLLM

from movie_rag.models import Budget, RecommendRequest, RetrievalResult, SearchHit, ServiceError
from movie_rag.service import RagService


class FakePipeline:
    def __init__(self, movies):
        self.movies = movies

    def search(self, query, filters, limit, strategy, budget):
        return RetrievalResult([SearchHit(movie, 1) for movie in self.movies], strategy)


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
    response = RagService(FakePipeline(movies), llm, Ready(), catalog, "dense").recommend(
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
    result = RagService(FakePipeline(movies), llm, Ready(), catalog, "dense", context_bytes=1).recommend(
        RecommendRequest(query="space"),
        Budget.seconds(10),
    )
    assert result.mode == "retrieval_only" and not llm.calls
    assert "CONTEXT_LIMIT" in [warning.code for warning in result.warnings]
    with pytest.raises(ServiceError):
        RagService(FakePipeline(movies), llm, Ready(), catalog).recommend(RecommendRequest(query="space"), Budget.seconds(-1))
