import hashlib
import json
from pathlib import Path
from typing import Literal

from pydantic import ValidationError, model_validator

from movie_rag.config import AppConfig
from movie_rag.embeddings import BM25Encoder, BM25State, HybridEmbedder
from movie_rag.models import Catalog, IndexSchema, Model, ServiceError
from movie_rag.ports import DenseEmbeddingModel, IndexStore
from movie_rag.preprocessing import MovieFormatter, load_movies


class Manifest(Model):
    version: Literal[2] = 2
    status: Literal["building", "ready"]
    index_id: str
    csv_sha256: str
    config: dict
    vector_schema: IndexSchema
    max_length: int | None = None
    bm25: BM25State | None = None
    catalog: Catalog
    truncated_documents: int = 0

    @model_validator(mode="after")
    def consistent_vectors(self) -> "Manifest":
        if ("bm25" in self.vector_schema.enabled_vectors) != (self.bm25 is not None):
            raise ValueError("BM25 statistics do not match the schema")
        if self.bm25 and self.catalog.movie_count != self.bm25.n:
            raise ValueError("Corpus count mismatch")
        if ("transformer" in self.vector_schema.enabled_vectors) != (self.max_length is not None):
            raise ValueError("Transformer metadata do not match the schema")
        return self

    def fingerprint(self) -> str:
        data = self.model_dump(exclude={"index_id", "status", "truncated_documents"})
        return hashlib.sha256(json.dumps(data, sort_keys=True, ensure_ascii=False).encode()).hexdigest()


def save_manifest(path: Path, manifest: Manifest) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(".tmp")
    temporary.write_text(manifest.model_dump_json(indent=2), encoding="utf-8")
    temporary.replace(path)


def load_manifest(path: Path, settings: AppConfig) -> Manifest:
    try:
        manifest = Manifest.model_validate_json(path.read_text(encoding="utf-8"))
        if (
            manifest.status != "ready"
            or manifest.config != settings.index_config()
            or manifest.fingerprint() != manifest.index_id
        ):
            raise ValueError("Incompatible manifest")
        if sorted(manifest.vector_schema.enabled_vectors) != sorted(settings.index.enabled_vectors):
            raise ValueError("Vector configuration mismatch")
        return manifest
    except (OSError, ValueError, ValidationError) as exc:
        raise ServiceError("UNAVAILABLE", "Missing or incompatible ready index; run import first") from exc


class IndexReadiness:
    def __init__(self, store: IndexStore, manifest: Manifest) -> None:
        self.store, self.manifest = store, manifest

    def check(self) -> None:
        self.store.verify(self.manifest.vector_schema, self.manifest.catalog.movie_count, self.manifest.index_id)


def import_index(settings: AppConfig, store: IndexStore, dense: DenseEmbeddingModel | None = None) -> Manifest:
    needs_dense = "transformer" in settings.index.enabled_vectors
    if needs_dense != (dense is not None):
        raise ValueError("Transformer dependency must match enabled_vectors")
    path = settings.index.state_dir / "manifest.json"
    checksum = hashlib.sha256(settings.index.dataset_path.read_bytes()).hexdigest()
    if path.exists():
        previous = load_manifest(path, settings)
        if previous.csv_sha256 != checksum:
            raise ValueError("Dataset changed; use a new qdrant.collection and index.state_dir")
        IndexReadiness(store, previous).check()
        return previous
    movies = load_movies(settings.index.dataset_path)
    formatter = MovieFormatter()
    texts = [formatter.format(movie) for movie in movies]
    sparse = None
    if "bm25" in settings.index.enabled_vectors:
        sparse = BM25Encoder(settings.bm25.k1, settings.bm25.b)
        sparse.fit(texts)
    genres = {genre.id: genre for movie in movies for genre in movie.genres}
    years = [movie.year for movie in movies if movie.year is not None]
    manifest = Manifest(
        status="building",
        index_id="",
        csv_sha256=checksum,
        config=settings.index_config(),
        vector_schema=IndexSchema(
            enabled_vectors=sorted(settings.index.enabled_vectors), dimension=dense.dimension if dense else None
        ),
        max_length=dense.max_length if dense else None,
        bm25=sparse.state if sparse else None,
        catalog=Catalog(
            genres=[genres[i] for i in sorted(genres)],
            movie_count=len(movies),
            year_from=min(years) if years else None,
            year_to=max(years) if years else None,
        ),
    )
    manifest.index_id = manifest.fingerprint()
    store.create(manifest.vector_schema)
    save_manifest(path, manifest)
    encoder = HybridEmbedder(sparse, dense) if sparse and dense else sparse or dense
    for offset in range(0, len(movies), settings.index.batch_size):
        batch = movies[offset : offset + settings.index.batch_size]
        vectors = encoder.encode_documents(texts[offset : offset + len(batch)])
        store.upsert(batch, vectors, manifest.index_id)
    store.verify(manifest.vector_schema, len(movies), manifest.index_id)
    manifest.truncated_documents = dense.truncated_documents if dense else 0
    manifest.status = "ready"
    save_manifest(path, manifest)
    return manifest
