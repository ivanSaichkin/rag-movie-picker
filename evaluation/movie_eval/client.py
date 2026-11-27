import sys
import time
from dataclasses import asdict
from pathlib import Path

import grpc
from google.protobuf.json_format import MessageToDict

from movie_eval.goldenset import GoldenQuery

RAG_SOURCE = Path(__file__).resolve().parents[2] / "recommender" / "src"
sys.path.insert(0, str(RAG_SOURCE))
# Use the repository's generated stubs without installing the full RAG service.
from rag.v1 import rag_pb2 as pb  # noqa: E402
from rag.v1 import rag_pb2_grpc as rpc  # noqa: E402


class RecommenderClient:
    def __init__(self, address: str, timeout: float) -> None:
        self.channel = grpc.insecure_channel(address, options=[("grpc.max_receive_message_length", 16 * 1024 * 1024)])
        self.stub = rpc.RagServiceStub(self.channel)
        self.timeout = timeout
        self.disable_llm = False
        self.inspect = False

    def close(self) -> None:
        self.channel.close()

    def catalog(self) -> dict:
        response = self.stub.GetCatalog(pb.GetCatalogRequest(), timeout=self.timeout)
        return {
            "movie_count": response.movie_count,
            "available_strategies": list(response.available_strategies),
            "default_strategy": response.default_strategy,
            "max_results": response.max_results,
            "genres": [{"id": genre.id, "name": genre.name} for genre in response.genres],
        }

    def recommend(self, case: GoldenQuery, strategy: str, k: int, generate: bool) -> dict:
        filters = {key: value for key, value in asdict(case.filters).items() if value is not None}
        request = pb.RecommendRequest(
            query=case.query,
            limit=k,
            strategy=strategy,
            generate_explanations=generate,
            disable_llm=self.disable_llm,
            filters=pb.Filters(**filters),
        )
        started = time.perf_counter()
        diagnostics = None
        try:
            if self.inspect:
                inspection = self.stub.InspectRecommendation(request, timeout=self.timeout)
                diagnostics = MessageToDict(
                    inspection.diagnostics, preserving_proto_field_name=True, always_print_fields_with_no_presence=True
                )
                if inspection.error_code:
                    return {
                        "query_id": case.query_id,
                        "query": case.query,
                        "filters": filters,
                        "requested_strategy": strategy,
                        "requested_k": k,
                        "generate_explanations": generate,
                        "latency_ms": (time.perf_counter() - started) * 1000,
                        "status": inspection.error_code,
                        "error": inspection.error_message,
                        "items": [],
                        "warnings": [],
                        "mode": None,
                        "strategy_used": None,
                        "diagnostics": diagnostics,
                    }
                response = inspection.response
            else:
                response = self.stub.Recommend(request, timeout=self.timeout)
            movies = []
            for item in response.items:
                movie = item.movie
                movies.append(
                    {
                        "movie_id": movie.movie_id,
                        "title": movie.title,
                        "overview": movie.overview,
                        "year": movie.year if movie.HasField("year") else None,
                        "rating": movie.rating if movie.HasField("rating") else None,
                        "genre_ids": [genre.id for genre in movie.genres],
                        "reason": item.reason if item.HasField("reason") else None,
                    }
                )
            result = {
                "status": "OK",
                "error": None,
                "mode": response.mode,
                "explanation": response.explanation if response.HasField("explanation") else None,
                "has_matches": response.has_matches if response.HasField("has_matches") else None,
                "strategy_used": response.strategy_used,
                "items": movies,
                "warnings": [{"code": warning.code, "message": warning.message} for warning in response.warnings],
            }
        except grpc.RpcError as error:
            result = {
                "status": error.code().name,
                "error": error.details(),
                "items": [],
                "warnings": [],
                "mode": None,
                "strategy_used": None,
            }
        return {
            "query_id": case.query_id,
            "query": case.query,
            "filters": filters,
            "requested_strategy": strategy,
            "requested_k": k,
            "generate_explanations": generate,
            "latency_ms": (time.perf_counter() - started) * 1000,
            **result,
            **({"diagnostics": diagnostics} if self.inspect else {}),
        }
