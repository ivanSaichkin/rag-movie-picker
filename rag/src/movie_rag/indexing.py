import hashlib
import json
from pathlib import Path
from typing import Literal

from pydantic import ValidationError

from movie_rag.config import Settings
from movie_rag.embeddings import BM25Encoder, BM25State, HybridEmbedder
from movie_rag.models import Catalog, Model, ServiceError
from movie_rag.ports import DenseEmbeddingModel, IndexStore
from movie_rag.preprocessing import MovieFormatter, load_movies


class Manifest(Model):
    status: Literal["building", "ready"]
    index_id: str
    csv_sha256: str
    config: dict
    dimension: int
    max_length: int
    bm25: BM25State
    catalog: Catalog
    truncated_documents: int = 0

    def fingerprint(self) -> str:
        data = self.model_dump(exclude={"index_id", "status", "truncated_documents"})
        return hashlib.sha256(json.dumps(data, sort_keys=True, ensure_ascii=False).encode()).hexdigest()


def save_manifest(path: Path, manifest: Manifest) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(".tmp")
    temporary.write_text(manifest.model_dump_json(indent=2), encoding="utf-8")
    temporary.replace(path)


def load_manifest(path: Path, settings: Settings) -> Manifest:
    try:
        manifest = Manifest.model_validate_json(path.read_text(encoding="utf-8"))
        if (
            manifest.status != "ready"
            or manifest.config != settings.index_config()
            or manifest.fingerprint() != manifest.index_id
        ):
            raise ValueError("Incompatible manifest")
        if manifest.catalog.movie_count != manifest.bm25.n:
            raise ValueError("Corpus count mismatch")
        return manifest
    except (OSError, ValueError, ValidationError) as exc:
        raise ServiceError("UNAVAILABLE", "Missing or incompatible ready index; run import first") from exc


class IndexReadiness:
    def __init__(self, store: IndexStore, manifest: Manifest) -> None:
        self.store, self.manifest = store, manifest

    def check(self) -> None:
        self.store.verify(self.manifest.dimension, self.manifest.catalog.movie_count, self.manifest.index_id)


def import_index(settings: Settings, store: IndexStore, dense: DenseEmbeddingModel) -> Manifest:
    path = settings.index_state_dir / "manifest.json"
    checksum = hashlib.sha256(settings.dataset_path.read_bytes()).hexdigest()
    if path.exists():
        previous = load_manifest(path, settings)
        if previous.csv_sha256 != checksum:
            raise ValueError("Dataset changed; use a new collection and INDEX_STATE_DIR")
        IndexReadiness(store, previous).check()
        return previous
    movies = load_movies(settings.dataset_path)
    formatter = MovieFormatter()
    texts = [formatter.format(movie) for movie in movies]
    sparse = BM25Encoder(settings.bm25_k1, settings.bm25_b)
    sparse.fit(texts)
    genres = {genre.id: genre for movie in movies for genre in movie.genres}
    years = [movie.year for movie in movies if movie.year is not None]
    manifest = Manifest(
        status="building",
        index_id="",
        csv_sha256=checksum,
        config=settings.index_config(),
        dimension=dense.dimension,
        max_length=dense.max_length,
        bm25=sparse.state,
        catalog=Catalog(
            genres=[genres[i] for i in sorted(genres)],
            movie_count=len(movies),
            year_from=min(years) if years else None,
            year_to=max(years) if years else None,
        ),
    )
    manifest.index_id = manifest.fingerprint()
    store.create(dense.dimension)
    save_manifest(path, manifest)
    encoder = HybridEmbedder(sparse, dense)
    for offset in range(0, len(movies), settings.embedding_batch_size):
        batch = movies[offset : offset + settings.embedding_batch_size]
        vectors = encoder.encode_documents(texts[offset : offset + len(batch)])
        store.upsert(batch, vectors, manifest.index_id)
    store.verify(manifest.dimension, len(movies), manifest.index_id)
    manifest.truncated_documents = dense.truncated_documents
    manifest.status = "ready"
    save_manifest(path, manifest)
    return manifest
