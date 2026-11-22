from contextlib import closing

import pytest
from conftest import FakeDense, ScriptedLLM
from qdrant_client import QdrantClient
from test_import import row, write_csv

from movie_rag.cli import create_transformer, run
from movie_rag.config import AppConfig
from movie_rag.embeddings import BM25Encoder
from movie_rag.indexing import IndexReadiness, import_index, load_manifest
from movie_rag.models import Budget, Filters, RecommendRequest, ServiceError, VectorRepresentation
from movie_rag.retrieval import RetrievalPipeline
from movie_rag.service import RagService
from movie_rag.storage import QdrantStore


@pytest.mark.integration
@pytest.mark.parametrize("vectors", [["bm25"], ["transformer"], ["bm25", "transformer"]])
def test_index_and_every_supported_strategy(tmp_path, vectors):
    path = tmp_path / "movies.csv"
    write_csv(path, [row(1, release_date="2014-01-01"), row(2, overview="Garden comedy", release_date="2001-01-01")])
    default = "hybrid" if len(vectors) == 2 else vectors[0]
    config = AppConfig(
        index={"enabled_vectors": vectors, "dataset_path": path, "state_dir": tmp_path / "state"},
        search={"default_strategy": default, "generate_explanations": False},
        bm25={"query_preparation": "none"},
        llm={"enabled": False},
    )
    dense = FakeDense() if "transformer" in vectors else None
    with closing(QdrantClient(":memory:")) as client:
        store = QdrantStore(client, config.qdrant.collection)
        manifest = import_index(config, store, dense)
        assert import_index(config, store, dense).index_id == manifest.index_id
        restored = load_manifest(config.index.state_dir / "manifest.json", config)
        assert restored.vector_schema.enabled_vectors == sorted(vectors)
        assert (restored.bm25 is not None) == ("bm25" in vectors)
        info = client.get_collection(config.qdrant.collection)
        assert set(info.config.params.vectors) == ({"semantic"} if dense else set())
        assert set(info.config.params.sparse_vectors or {}) == ({"bm25"} if "bm25" in vectors else set())
        points, _ = client.scroll(config.qdrant.collection, with_vectors=True)
        expected = ({"semantic"} if dense else set()) | ({"bm25"} if "bm25" in vectors else set())
        assert all(set(point.vector) == expected for point in points)
        sparse = BM25Encoder(state=restored.bm25) if restored.bm25 else None
        pipeline = RetrievalPipeline(sparse, dense, store)
        llm = ScriptedLLM([])
        catalog = restored.catalog.model_copy(
            update={"available_strategies": config.available_strategies, "default_strategy": default}
        )
        service = RagService(pipeline, llm, IndexReadiness(store, restored), catalog, config.search, config.bm25, config.llm)
        for strategy in config.available_strategies:
            result = service.recommend(
                RecommendRequest(query="space", limit=1, strategy=strategy, filters=Filters(year_from=2010)), Budget.seconds(10)
            )
            assert result.mode == "retrieval_only"
            assert result.strategy_used == strategy
            assert [item.movie.movie_id for item in result.items] == [1]
            assert result.items[0].reason is None
            assert not result.warnings
        for strategy in {"bm25", "transformer", "hybrid"} - set(config.available_strategies):
            with pytest.raises(ServiceError, match="not present"):
                service.recommend(RecommendRequest(query="space", strategy=strategy), Budget.seconds(10))
        assert llm.calls == []
        incomplete = (
            VectorRepresentation(sparse={"indices": [1], "values": [1]}) if dense else VectorRepresentation(dense=[1, 0, 0])
        )
        with pytest.raises(ValueError, match="schema"):
            store.upsert([store.get(1)], [incomplete], manifest.index_id)


def test_bm25_does_not_construct_transformer(monkeypatch):
    def forbidden(*args, **kwargs):
        raise AssertionError("Transformer must not be loaded")

    monkeypatch.setattr("movie_rag.cli.TransformerEmbedder", forbidden)
    config = AppConfig(index={"enabled_vectors": ["bm25"]}, search={"default_strategy": "bm25"})
    assert create_transformer(config) is None


@pytest.mark.integration
def test_bm25_cli_import_without_transformer_or_network(tmp_path, monkeypatch):
    def forbidden(*args, **kwargs):
        raise AssertionError("BM25 import must not use a transformer or LLM")

    monkeypatch.setattr("movie_rag.cli.TransformerEmbedder", forbidden)
    monkeypatch.setattr("movie_rag.cli.OpenRouterClient", forbidden)
    path = tmp_path / "movies.csv"
    write_csv(path, [row()])
    config = AppConfig(
        index={"enabled_vectors": ["bm25"], "dataset_path": path, "state_dir": tmp_path / "state"},
        search={"default_strategy": "bm25"},
        qdrant={"path": tmp_path / "qdrant"},
    )
    run(config, "import")
    assert load_manifest(config.index.state_dir / "manifest.json", config).catalog.movie_count == 1


@pytest.mark.integration
def test_changed_vector_set_rejects_existing_index(tmp_path):
    path = tmp_path / "movies.csv"
    write_csv(path, [row()])
    config = AppConfig(
        index={"enabled_vectors": ["bm25"], "dataset_path": path, "state_dir": tmp_path / "state"},
        search={"default_strategy": "bm25"},
    )
    with closing(QdrantClient(":memory:")) as client:
        store = QdrantStore(client, config.qdrant.collection)
        manifest = import_index(config, store)
        changed = AppConfig(index={"dataset_path": path, "state_dir": config.index.state_dir})
        with pytest.raises(ServiceError, match="incompatible"):
            import_index(changed, store, FakeDense())
        store.verify(manifest.vector_schema, 1, manifest.index_id)
