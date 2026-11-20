import json

import grpc
import httpx
import pytest
from conftest import Ready

from movie_rag.config import BM25Config, LLMConfig, SearchConfig
from movie_rag.llm import OpenRouterClient
from movie_rag.models import Budget, RecommendRequest, RetrievalResult, SearchHit, ServiceError
from movie_rag.service import RagService
from movie_rag.transport import RagHandler, create_server
from rag.v1 import rag_pb2 as pb
from rag.v1 import rag_pb2_grpc as rpc


class Candidates:
    def __init__(self, movies):
        self.movies = movies
        self.limit = None

    def search(self, query, filters, limit, strategy, budget):
        self.limit = limit
        return RetrievalResult([SearchHit(movie, 1) for movie in self.movies[:limit]], strategy)


def test_candidates_before_after_and_raw_llm_trace(movies, catalog):
    bodies = []

    def respond(request):
        bodies.append(json.loads(request.content))
        return httpx.Response(200, json={"choices": [{"message": {"content": '{"items":[{"movie_id":2,"reason":"Выбран."}]}'}}]})

    with httpx.Client(transport=httpx.MockTransport(respond)) as http:
        pipeline = Candidates(movies)
        llm = OpenRouterClient(http, "https://example.test/v1", "test-secret", "test-model")
        service = RagService(pipeline, llm, Ready(), catalog, SearchConfig(default_strategy="transformer"))
        response = service.recommend(
            RecommendRequest(query="space", limit=1, candidate_limit=3, include_diagnostics=True), Budget.seconds(10)
        )
    assert pipeline.limit == 3
    assert [candidate.movie.movie_id for candidate in response.diagnostics.candidates] == [1, 2, 3]
    assert response.diagnostics.llm_candidate_ids == [1, 2, 3]
    assert [item.movie.movie_id for item in response.items] == [2]
    trace = response.diagnostics.llm_calls[0]
    assert json.loads(trace.request_json) == bodies[0]
    assert json.loads(trace.attempts[0].content)["items"][0]["movie_id"] == 2
    assert "test-secret" not in response.diagnostics.model_dump_json()
    assert trace.attempts[0].http_status == 200
    assert response.diagnostics.timings_ms["rag_total"] >= trace.elapsed_ms > 0


@pytest.mark.parametrize("candidate_limit", [0, 1, 1001])
def test_invalid_candidate_limit(movies, catalog, candidate_limit):
    service = RagService(Candidates(movies), None, Ready(), catalog)
    with pytest.raises((ServiceError, ValueError)):
        service.recommend(RecommendRequest(query="x", limit=2, candidate_limit=candidate_limit), Budget.seconds(10))


@pytest.mark.integration
def test_inspection_preserves_failed_translation_and_regular_rpc_still_errors(movies, catalog):
    def respond(request):
        return httpx.Response(401, text='{"error":"test-secret invalid"}')

    with httpx.Client(transport=httpx.MockTransport(respond)) as http:
        llm = OpenRouterClient(http, "https://example.test", "test-secret", "model")
        service = RagService(Candidates(movies), llm, Ready(), catalog, SearchConfig(default_strategy="bm25"), BM25Config())
        server = create_server(RagHandler(service, None, Ready(), catalog, 10), Ready())
        port = server.add_insecure_port("127.0.0.1:0")
        server.start()
        try:
            with grpc.insecure_channel(f"127.0.0.1:{port}") as channel:
                stub = rpc.RagServiceStub(channel)
                inspection = stub.InspectRecommendation(pb.RecommendRequest(query="space", limit=1), timeout=5)
                assert inspection.error_code == "UNAVAILABLE"
                attempt = inspection.diagnostics.llm_calls[0].attempts[0]
                assert attempt.http_status == 401 and "[REDACTED]" in attempt.provider_response
                assert not inspection.HasField("response")
                with pytest.raises(grpc.RpcError):
                    stub.Recommend(pb.RecommendRequest(query="space", limit=1), timeout=5)
        finally:
            server.stop(0).wait()


def test_bad_json_and_retries_remain_visible(movies, catalog):
    calls = []

    def respond(request):
        calls.append(request)
        if len(calls) == 1:
            return httpx.Response(503, text="temporary failure")
        return httpx.Response(200, json={"choices": [{"message": {"content": "not JSON"}}]})

    with httpx.Client(transport=httpx.MockTransport(respond)) as http:
        service = RagService(
            Candidates(movies),
            OpenRouterClient(http, "https://example.test", "secret", "model"),
            Ready(),
            catalog,
            SearchConfig(default_strategy="transformer"),
        )
        result = service.recommend(RecommendRequest(query="space", limit=1, include_diagnostics=True), Budget.seconds(10))
    assert result.mode == "retrieval_only"
    attempts = result.diagnostics.llm_calls[0].attempts
    assert [attempt.http_status for attempt in attempts] == [503, 200]
    assert attempts[-1].content == "not JSON"
    assert attempts[-1].error == "JSONDecodeError"


def test_normal_response_has_no_diagnostics(movies, catalog):
    service = RagService(Candidates(movies), None, Ready(), catalog, SearchConfig(default_strategy="transformer"))
    result = service.recommend(RecommendRequest(query="space", generate_explanations=False), Budget.seconds(10))
    assert result.diagnostics is None


@pytest.mark.integration
def test_successful_inspection_returns_context_trimmed_candidates(movies, catalog):
    movies[1] = movies[1].model_copy(update={"overview": "x" * 5000})
    bodies = []

    def respond(request):
        bodies.append(json.loads(request.content))
        return httpx.Response(200, json={"choices": [{"message": {"content": '{"items":[{"movie_id":1,"reason":"Подходит"}]}'}}]})

    with httpx.Client(transport=httpx.MockTransport(respond)) as http:
        llm = OpenRouterClient(http, "https://example.test", "secret", "model")
        service = RagService(
            Candidates(movies),
            llm,
            Ready(),
            catalog,
            SearchConfig(default_strategy="transformer"),
            generation=LLMConfig(context_bytes=2048),
        )
        server = create_server(RagHandler(service, None, Ready(), catalog, 10), Ready())
        port = server.add_insecure_port("127.0.0.1:0")
        server.start()
        try:
            with grpc.insecure_channel(f"127.0.0.1:{port}") as channel:
                result = rpc.RagServiceStub(channel).InspectRecommendation(
                    pb.RecommendRequest(query="space", limit=1, candidate_limit=3), timeout=5
                )
            assert not result.error_code
            assert result.response.mode == "generated"
            assert len(result.diagnostics.candidates) == 3
            assert list(result.diagnostics.llm_candidate_ids) == [1]
            assert len(json.loads(bodies[0]["messages"][1]["content"])["candidates"]) == 1
            assert result.response.items[0].movie.movie_id == 1
            assert result.diagnostics.timings_ms["rag_total"] > 0
        finally:
            server.stop(0).wait()
