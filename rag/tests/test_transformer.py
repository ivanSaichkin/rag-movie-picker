import os
import sys
from types import SimpleNamespace

import numpy as np
import pytest

from movie_rag.embeddings import TransformerEmbedder
from movie_rag.models import ServiceError


def test_transformer_prefixes_normalization_and_truncation(monkeypatch):
    calls = []

    class Model:
        max_seq_length = 3

        def get_embedding_dimension(self):
            return 2

        def tokenizer(self, texts, **kwargs):
            return {"input_ids": [[1, 2, 3, 4] for _ in texts]}

        def encode(self, texts, **kwargs):
            calls.append((texts, kwargs))
            return np.array([[1.0, 0.0] for _ in texts])

    monkeypatch.setitem(sys.modules, "sentence_transformers", SimpleNamespace(SentenceTransformer=lambda *a, **kw: Model()))
    encoder = TransformerEmbedder("test", "revision")
    assert len(encoder.encode_documents(["one", "two"])) == 2
    encoder.encode_query("space")
    assert calls[0][0] == ["passage: one", "passage: two"]
    assert calls[1][0] == ["query: space"]
    assert calls[0][1]["normalize_embeddings"] is True
    assert encoder.truncated_documents == 2

    def fail(*args, **kwargs):
        raise RuntimeError("device failed")

    encoder.model.encode = fail
    with pytest.raises(ServiceError):
        encoder.encode_query("space")


@pytest.mark.model
@pytest.mark.skipif(os.environ.get("RUN_MODEL_TESTS") != "1", reason="Opt-in locally cached transformer test")
def test_real_cached_transformer():
    encoder = TransformerEmbedder(
        "intfloat/multilingual-e5-small",
        "614241f622f53c4eeff9890bdc4f31cfecc418b3",
        local_only=True,
    )
    documents = encoder.encode_documents(
        [
            "Astronauts explore distant planets aboard a spaceship.",
            "A family runs a restaurant and cooks traditional food.",
        ]
    )
    query = encoder.encode_query("Космическое путешествие к другим планетам").dense
    assert len(query) == encoder.dimension
    assert np.linalg.norm(query) == pytest.approx(1, abs=1e-5)
    assert np.dot(query, documents[0].dense) > np.dot(query, documents[1].dense)

    # Exercise the same real model through Qdrant, RRF and the public gRPC API.
    import grpc
    from conftest import Ready, ScriptedLLM
    from qdrant_client import QdrantClient

    from movie_rag.embeddings import BM25Encoder, HybridEmbedder
    from movie_rag.models import Catalog, Movie
    from movie_rag.preprocessing import MovieFormatter
    from movie_rag.retrieval import RetrievalPipeline
    from movie_rag.service import RagService
    from movie_rag.storage import QdrantStore
    from movie_rag.transport import RagHandler, create_server
    from rag.v1 import rag_pb2 as pb
    from rag.v1 import rag_pb2_grpc as rpc

    movies = [
        Movie(movie_id=1, title="Space", overview="Astronauts explore distant planets aboard a spaceship."),
        Movie(movie_id=2, title="Restaurant", overview="A family runs a restaurant and cooks traditional food."),
    ]
    texts = [MovieFormatter().format(movie) for movie in movies]
    sparse = BM25Encoder()
    sparse.fit(texts)
    client = QdrantClient(":memory:")
    server = None
    try:
        store = QdrantStore(client, "model_smoke")
        store.create(encoder.dimension)
        store.upsert(movies, HybridEmbedder(sparse, encoder).encode_documents(texts), "smoke")
        catalog = Catalog(genres=[], movie_count=2)
        llm = ScriptedLLM([{"search_query_en": "space planets spaceship"}, ServiceError("UNAVAILABLE", "offline")])
        service = RagService(RetrievalPipeline(sparse, encoder, store), llm, Ready(), catalog)
        server = create_server(RagHandler(service, store, Ready(), catalog, 30), Ready())
        port = server.add_insecure_port("127.0.0.1:0")
        server.start()
        with grpc.insecure_channel(f"127.0.0.1:{port}") as channel:
            result = rpc.RagServiceStub(channel).Recommend(
                pb.RecommendRequest(query="Космическое путешествие к другим планетам", limit=1),
                timeout=30,
            )
            assert result.mode == "retrieval_only"
            assert result.strategy_used == "hybrid"
            assert result.items[0].movie.movie_id == 1
    finally:
        if server is not None:
            server.stop(0).wait()
        client.close()
