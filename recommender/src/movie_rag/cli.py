import argparse
import logging
from contextlib import ExitStack
from pathlib import Path

import httpx
from filelock import FileLock
from qdrant_client import QdrantClient

from movie_rag.config import AppConfig, load_config
from movie_rag.credentials import read_api_key
from movie_rag.embeddings import BM25Encoder, TransformerEmbedder
from movie_rag.indexing import IndexReadiness, Manifest, import_index, load_manifest
from movie_rag.llm import OpenRouterClient
from movie_rag.models import ServiceError
from movie_rag.ports import DenseEmbeddingModel, IndexStore
from movie_rag.retrieval import RetrievalPipeline
from movie_rag.service import RagService
from movie_rag.storage import QdrantStore
from movie_rag.title_lookup import CatalogTitleLookup
from movie_rag.transport import RagHandler, create_server


def run(settings: AppConfig, command: str, env_file: Path | None = None) -> None:
    settings.index.state_dir.mkdir(parents=True, exist_ok=True)
    # Import and serving the same index are mutually exclusive in the local MVP.
    with ExitStack() as stack:
        stack.enter_context(FileLock(str(settings.index.state_dir / "index.lock"), timeout=0))
        client = (
            QdrantClient(path=str(settings.qdrant.path))
            if settings.qdrant.path
            else QdrantClient(url=settings.qdrant.url, timeout=5)
        )
        stack.callback(client.close)
        store = QdrantStore(client, settings.qdrant.collection)
        manifest = None if command == "import" else load_manifest(settings.index.state_dir / "manifest.json", settings)
        dense = create_transformer(settings)
        if command == "import":
            manifest = import_index(settings, store, dense)
            print(f"Index ready: movies={manifest.catalog.movie_count}, truncated={manifest.truncated_documents}")
            return
        serve(settings, store, manifest, dense, stack, env_file)


def create_transformer(settings: AppConfig) -> DenseEmbeddingModel | None:
    if "transformer" not in settings.index.enabled_vectors:
        return None
    model = settings.transformer
    return TransformerEmbedder(model.model, model.revision, model.device, model.batch_size, model.local_only)


def serve(
    settings: AppConfig,
    store: IndexStore,
    manifest: Manifest,
    dense: DenseEmbeddingModel | None,
    stack: ExitStack,
    env_file: Path | None = None,
) -> None:
    if dense and (dense.dimension != manifest.vector_schema.dimension or dense.max_length != manifest.max_length):
        raise ServiceError("UNAVAILABLE", "Model and manifest mismatch")
    readiness = IndexReadiness(store, manifest)
    readiness.check()
    catalog = manifest.catalog.model_copy(
        update={
            "default_results": settings.search.default_results,
            "max_results": settings.search.max_results,
            "available_strategies": settings.available_strategies,
            "default_strategy": settings.search.default_strategy,
            "max_candidates": settings.search.max_candidates,
        }
    )
    http = stack.enter_context(httpx.Client())
    llm = settings.llm
    adapter = OpenRouterClient(
        http,
        llm.base_url,
        read_api_key(llm.api_key_env, env_file) if llm.enabled else "",
        llm.model,
        llm.timeout_seconds,
        llm.max_output_tokens,
        reasoning_enabled=llm.reasoning_enabled,
        structured_outputs=llm.structured_outputs,
    )
    sparse = BM25Encoder(state=manifest.bm25) if manifest.bm25 is not None else None
    pipeline = RetrievalPipeline(sparse, dense, store, settings.search.rrf_k)
    titles = CatalogTitleLookup.from_csv(settings.index.dataset_path, manifest.csv_sha256)
    service = RagService(pipeline, adapter, readiness, catalog, settings.search, settings.bm25, settings.llm, titles)
    server = create_server(
        RagHandler(service, store, readiness, catalog, settings.server.request_timeout_seconds),
        readiness,
        settings.server.workers,
    )
    if server.add_insecure_port(settings.server.address) == 0:
        raise RuntimeError("Cannot bind gRPC address")
    server.start()
    logging.info("RAG listening on %s", settings.server.address)
    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        pass
    finally:
        server.stop(grace=5).wait()


def main() -> None:
    parser = argparse.ArgumentParser(description="Movie Picker RAG")
    parser.add_argument("command", choices=["import", "serve"])
    parser.add_argument("--config", type=Path, default=Path("config.toml"))
    parser.add_argument("--env-file", type=Path, help="Secret file; defaults to .env beside the selected TOML")
    args = parser.parse_args()
    logging.basicConfig(level=logging.INFO, format="%(levelname)s %(message)s")
    env_file = args.env_file.resolve() if args.env_file else args.config.resolve().parent / ".env"
    run(load_config(args.config), args.command, env_file)


if __name__ == "__main__":
    main()
