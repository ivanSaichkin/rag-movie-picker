import logging
import time
import uuid
from collections.abc import Callable
from concurrent import futures
from typing import TypeVar

import grpc
from grpc_health.v1 import health_pb2, health_pb2_grpc
from pydantic import ValidationError

from movie_rag.models import Budget, Catalog, Filters, Movie, RecommendRequest, ServiceError
from movie_rag.ports import Readiness, Recommender, VectorStore
from rag.v1 import rag_pb2 as pb
from rag.v1 import rag_pb2_grpc as rpc

Response = TypeVar("Response")


def movie_message(movie: Movie) -> pb.Movie:
    message = pb.Movie(
        movie_id=movie.movie_id,
        title=movie.title,
        genres=[pb.Genre(id=g.id, name=g.name) for g in movie.genres],
        vote_count=movie.vote_count,
        overview=movie.overview,
    )
    if movie.year is not None:
        message.year = movie.year
    if movie.rating is not None:
        message.rating = movie.rating
    return message


class RagHandler(rpc.RagServiceServicer):
    def __init__(self, service: Recommender, store: VectorStore, readiness: Readiness, catalog: Catalog, timeout: float) -> None:
        self.service, self.store, self.readiness, self.catalog, self.timeout = service, store, readiness, catalog, timeout

    def _call(self, context: grpc.ServicerContext, action: Callable[[], Response]) -> Response:
        started = time.monotonic()
        # Metadata is not trusted log content; accept only a UUID.
        raw = dict(context.invocation_metadata()).get("request_id", "")
        try:
            request_id = str(uuid.UUID(raw))
        except (ValueError, TypeError):
            request_id = str(uuid.uuid4())
        context.send_initial_metadata((("request_id", request_id),))
        code = "OK"
        try:
            return action()
        except ValidationError:
            code = "INVALID_ARGUMENT"
            context.abort(grpc.StatusCode.INVALID_ARGUMENT, "Invalid request")
        except ServiceError as exc:
            code = exc.code
            context.abort(getattr(grpc.StatusCode, code, grpc.StatusCode.INTERNAL), str(exc))
        except Exception:
            code = "INTERNAL"
            context.abort(grpc.StatusCode.INTERNAL, "Internal service error")
        finally:
            logging.info("rpc request_id=%s code=%s duration_ms=%.1f", request_id, code, (time.monotonic() - started) * 1000)

    def Recommend(self, request: pb.RecommendRequest, context: grpc.ServicerContext) -> pb.RecommendResponse:
        def action():
            filters = request.filters
            domain = RecommendRequest(
                query=request.query.strip(),
                limit=request.limit if request.HasField("limit") else self.catalog.default_results,
                filters=Filters(
                    genre_ids=list(filters.genre_ids),
                    year_from=filters.year_from if filters.HasField("year_from") else None,
                    year_to=filters.year_to if filters.HasField("year_to") else None,
                    min_rating=filters.min_rating if filters.HasField("min_rating") else None,
                ),
            )
            remaining = context.time_remaining()
            budget = Budget.seconds(min(self.timeout, remaining) if remaining is not None else self.timeout, context.is_active)
            result = self.service.recommend(domain, budget)
            items = []
            for item in result.items:
                message = pb.Recommendation(movie=movie_message(item.movie))
                if item.reason is not None:
                    message.reason = item.reason
                items.append(message)
            return pb.RecommendResponse(
                mode=result.mode,
                strategy_used=result.strategy_used,
                items=items,
                warnings=[pb.Warning(code=w.code, message=w.message) for w in result.warnings],
            )

        return self._call(context, action)

    def GetMovie(self, request: pb.GetMovieRequest, context: grpc.ServicerContext) -> pb.Movie:
        def action():
            if request.movie_id <= 0:
                raise ServiceError("INVALID_ARGUMENT", "Invalid movie ID")
            self.readiness.check()
            movie = self.store.get(request.movie_id)
            if movie is None:
                raise ServiceError("NOT_FOUND", "Movie not found")
            return movie_message(movie)

        return self._call(context, action)

    def GetCatalog(self, request: pb.GetCatalogRequest, context: grpc.ServicerContext) -> pb.Catalog:
        def action():
            self.readiness.check()
            result = pb.Catalog(
                genres=[pb.Genre(id=g.id, name=g.name) for g in self.catalog.genres],
                movie_count=self.catalog.movie_count,
                default_results=self.catalog.default_results,
                max_results=self.catalog.max_results,
            )
            if self.catalog.year_from is not None:
                result.year_from = self.catalog.year_from
            if self.catalog.year_to is not None:
                result.year_to = self.catalog.year_to
            return result

        return self._call(context, action)


class HealthHandler(health_pb2_grpc.HealthServicer):
    def __init__(self, readiness: Readiness) -> None:
        self.readiness = readiness

    def Check(self, request: health_pb2.HealthCheckRequest, context: grpc.ServicerContext) -> health_pb2.HealthCheckResponse:
        if request.service not in {"", "rag.v1.RagService"}:
            context.abort(grpc.StatusCode.NOT_FOUND, "Unknown service")
        try:
            self.readiness.check()
            return health_pb2.HealthCheckResponse(status=health_pb2.HealthCheckResponse.SERVING)
        except ServiceError:
            return health_pb2.HealthCheckResponse(status=health_pb2.HealthCheckResponse.NOT_SERVING)


def create_server(handler: RagHandler, readiness: Readiness, workers: int = 4) -> grpc.Server:
    # Synchronous handlers run in bounded worker threads, not an asyncio event loop.
    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=workers),
        maximum_concurrent_rpcs=workers,
        options=[("grpc.max_receive_message_length", 64 * 1024), ("grpc.max_send_message_length", 8 * 1024 * 1024)],
    )
    rpc.add_RagServiceServicer_to_server(handler, server)
    health_pb2_grpc.add_HealthServicer_to_server(HealthHandler(readiness), server)
    return server
