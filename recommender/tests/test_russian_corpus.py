import csv
import json
from contextlib import closing

import pytest
from qdrant_client import QdrantClient
from test_import import row, write_csv

from movie_rag.config import AppConfig
from movie_rag.embeddings import BM25Encoder
from movie_rag.indexing import import_index, load_manifest
from movie_rag.models import Budget, Filters, ServiceError
from movie_rag.storage import QdrantStore
from movie_rag.title_lookup import CatalogTitleLookup
from scripts.prepare_russian_dataset import prepare


def test_enrichment_joins_by_id_and_preserves_fallback_and_ratings(tmp_path):
    source, enrichment, destination = (tmp_path / name for name in ("source.csv", "enriched.json", "ru.csv"))
    write_csv(source, [row(2, title="Untranslated", vote_count=10, vote_average=7), row(1, title="Space")])
    enrichment.write_text(
        json.dumps({"movies": [{"movie_id": 1, "title_ru": "Космос", "overview_ru": "Полёт."}]}), encoding="utf-8"
    )
    stats = prepare(source, enrichment, destination)
    with destination.open(encoding="utf-8", newline="") as stream:
        rows = list(csv.DictReader(stream))
    assert rows[0]["title"] == "Untranslated" and rows[0]["vote_average"] == "7"
    assert rows[1]["title"] == "Космос" and rows[1]["overview"] == "Полёт."
    assert stats["russian_overviews"] == 1 and stats["fallback_ids"] == [2]
    with pytest.raises(ValueError, match="overwrite"):
        prepare(source, enrichment, source)


def test_russian_index_roundtrip_and_title_checksum(tmp_path):
    path = tmp_path / "movies.csv"
    write_csv(path, [row(1, title="Космос", overview="Приключения космонавтов"), row(2, title="Сад", overview="Садовник")])
    settings = AppConfig(
        index={"enabled_vectors": ["bm25"], "dataset_path": path, "state_dir": tmp_path / "state"},
        search={"default_strategy": "bm25"},
        qdrant={"collection": "ru_test"},
        bm25={"tokenizer": "snowball-ru-en-v1", "query_preparation": "none"},
    )
    with closing(QdrantClient(":memory:")) as client:
        store = QdrantStore(client, "ru_test")
        manifest = import_index(settings, store)
        restored = load_manifest(settings.index.state_dir / "manifest.json", settings)
        encoder = BM25Encoder(state=restored.bm25)
        hits = store.search_sparse(encoder.encode_query("космонавт").sparse, Filters(), 10, Budget.seconds(10))
        assert [hit.movie.movie_id for hit in hits] == [1]
        assert CatalogTitleLookup.from_csv(path, manifest.csv_sha256).find("КОСМОС", Filters())[0].movie_id == 1
        path.write_text("changed", encoding="utf-8")
        with pytest.raises(ServiceError, match="differs"):
            CatalogTitleLookup.from_csv(path, manifest.csv_sha256)
