import grpc
import pytest
from conftest import Ready
from grpc_health.v1 import health_pb2, health_pb2_grpc

from movie_rag.models import Recommendation, RecommendResponse
from movie_rag.transport import RagHandler, create_server
from rag.v1 import rag_pb2 as pb
from rag.v1 import rag_pb2_grpc as rpc


class FakeService:
    def __init__(self, movie):
        self.movie = movie
        self.last_request = None

    def recommend(self, request, budget):
        budget.remaining()
        self.last_request = request
        return RecommendResponse(mode="retrieval_only", strategy_used="dense", items=[Recommendation(movie=self.movie)])


class FakeStore:
    def __init__(self, movie):
        self.movie = movie

    def get(self, movie_id):
        return self.movie if movie_id == self.movie.movie_id else None


@pytest.mark.integration
def test_grpc_roundtrip_and_presence(movies, catalog):
    service = FakeService(movies[2])
    server = create_server(RagHandler(service, FakeStore(movies[2]), Ready(), catalog, 10), Ready())
    port = server.add_insecure_port("127.0.0.1:0")
    server.start()
    try:
        with grpc.insecure_channel(f"127.0.0.1:{port}") as channel:
            stub = rpc.RagServiceStub(channel)
            result = stub.Recommend(pb.RecommendRequest(query="Space", filters=pb.Filters(min_rating=0)), timeout=5)
            assert result.mode == "retrieval_only"
            assert not result.items[0].HasField("reason")
            assert not result.items[0].movie.HasField("rating")
            assert service.last_request.limit == catalog.default_results
            assert service.last_request.filters.min_rating == 0
            assert stub.GetCatalog(pb.GetCatalogRequest(), timeout=5).movie_count == 3
            assert stub.GetMovie(pb.GetMovieRequest(movie_id=3), timeout=5).movie_id == 3
            with pytest.raises(grpc.RpcError) as error:
                stub.GetMovie(pb.GetMovieRequest(movie_id=99), timeout=5)
            assert error.value.code() == grpc.StatusCode.NOT_FOUND
            for request in [pb.RecommendRequest(query=" "), pb.RecommendRequest(query="space", limit=0)]:
                with pytest.raises(grpc.RpcError) as error:
                    stub.Recommend(request, timeout=5)
                assert error.value.code() == grpc.StatusCode.INVALID_ARGUMENT
            health = health_pb2_grpc.HealthStub(channel)
            assert health.Check(health_pb2.HealthCheckRequest(service="rag.v1.RagService"), timeout=5).status == 1
    finally:
        server.stop(0).wait()


@pytest.mark.integration
def test_grpc_deadline(movies, catalog):
    import time

    class SlowService:
        def recommend(self, request, budget):
            time.sleep(0.1)
            budget.remaining()
            raise AssertionError("Expired request must not reach this point")

    server = create_server(RagHandler(SlowService(), FakeStore(movies[0]), Ready(), catalog, 10), Ready())
    port = server.add_insecure_port("127.0.0.1:0")
    server.start()
    try:
        with grpc.insecure_channel(f"127.0.0.1:{port}") as channel:
            grpc.channel_ready_future(channel).result(timeout=5)
            with pytest.raises(grpc.RpcError) as error:
                rpc.RagServiceStub(channel).Recommend(pb.RecommendRequest(query="space"), timeout=0.02)
            assert error.value.code() == grpc.StatusCode.DEADLINE_EXCEEDED
    finally:
        server.stop(0).wait()
