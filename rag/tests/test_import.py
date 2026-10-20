import csv
import json

import pytest
from conftest import FakeDense
from qdrant_client import QdrantClient

from movie_rag.config import Settings
from movie_rag.indexing import import_index, load_manifest
from movie_rag.models import ServiceError
from movie_rag.preprocessing import load_movies
from movie_rag.storage import QdrantStore


def write_csv(path, rows):
    fields = ["id", "title", "overview", "genres", "keywords", "release_date", "vote_average", "vote_count"]
    with path.open("w", encoding="utf-8", newline="") as stream:
        writer = csv.DictWriter(stream, fieldnames=fields)
        writer.writeheader()
        writer.writerows(rows)


def row(movie_id=1, **changes):
    return {
        "id": movie_id,
        "title": "Space",
        "overview": "Space adventure",
        "genres": "[]",
        "keywords": "[]",
        "release_date": "",
        "vote_average": "0",
        "vote_count": "0",
        **changes,
    }


def test_import_validation_and_duplicates(tmp_path):
    path = tmp_path / "movies.csv"
    write_csv(path, [row(), row(), row(2, overview=""), row(3, genres="not json")])
    movies = load_movies(path)
    assert len(movies) == 1 and movies[0].rating is None
    write_csv(path, [row(), row(title="Other")])
    with pytest.raises(ValueError, match="Conflicting"):
        load_movies(path)


@pytest.mark.integration
def test_import_idempotent_and_manifest_guard(tmp_path):
    path = tmp_path / "movies.csv"
    write_csv(path, [row(), row(2, overview="Garden comedy")])
    settings = Settings(_env_file=None, dataset_path=path, index_state_dir=tmp_path / "state")
    client = QdrantClient(":memory:")
    store = QdrantStore(client, settings.qdrant_collection)
    try:
        manifest = import_index(settings, store, FakeDense())
        again = import_index(settings, store, FakeDense())
        assert again.index_id == manifest.index_id
        assert client.count(settings.qdrant_collection).count == 2
        manifest_path = settings.index_state_dir / "manifest.json"
        assert load_manifest(manifest_path, settings).status == "ready"
        tampered = json.loads(manifest_path.read_text(encoding="utf-8"))
        tampered["bm25"]["avgdl"] = 99
        manifest_path.write_text(json.dumps(tampered), encoding="utf-8")
        with pytest.raises(ServiceError):
            load_manifest(manifest_path, settings)
    finally:
        client.close()


def test_config_limits():
    with pytest.raises(ValueError):
        Settings(_env_file=None, default_results=20, max_results=10)


@pytest.mark.integration
def test_failed_import_never_becomes_ready(tmp_path):
    class FailingDense(FakeDense):
        def encode_documents(self, texts):
            raise RuntimeError("Inference failed")

    path = tmp_path / "movies.csv"
    write_csv(path, [row()])
    settings = Settings(_env_file=None, dataset_path=path, index_state_dir=tmp_path / "state")
    client = QdrantClient(":memory:")
    try:
        with pytest.raises(RuntimeError):
            import_index(settings, QdrantStore(client, settings.qdrant_collection), FailingDense())
        with pytest.raises(ServiceError):
            load_manifest(settings.index_state_dir / "manifest.json", settings)
    finally:
        client.close()
