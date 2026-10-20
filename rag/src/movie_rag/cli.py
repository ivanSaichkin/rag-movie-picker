import argparse
import logging
from contextlib import ExitStack

import httpx
from filelock import FileLock
from qdrant_client import QdrantClient

from movie_rag.config import Settings
from movie_rag.embeddings import BM25Encoder, TransformerEmbedder
from movie_rag.indexing import IndexReadiness, import_index, load_manifest
from movie_rag.llm import OpenRouterClient
from movie_rag.models import ServiceError
from movie_rag.retrieval import RetrievalPipeline
from movie_rag.service import RagService
from movie_rag.storage import QdrantStore


def run(settings: Settings, command: str) -> None:
    settings.index_state_dir.mkdir(parents=True, exist_ok=True)
    # Import and serving the same index are mutually exclusive in the local MVP.
    with ExitStack() as stack:
        stack.enter_context(FileLock(str(settings.index_state_dir / "index.lock"), timeout=0))
        client = (
            QdrantClient(path=settings.qdrant_path) if settings.qdrant_path else QdrantClient(url=settings.qdrant_url, timeout=5)
        )
        stack.callback(client.close)
        store = QdrantStore(client, settings.qdrant_collection)
        manifest = None if command == "import" else load_manifest(settings.index_state_dir / "manifest.json", settings)
        dense = None
        if command == "import" or settings.retrieval_strategy != "sparse":
            dense = TransformerEmbedder(
                settings.embedding_model,
                settings.embedding_revision,
                settings.embedding_device,
                settings.embedding_batch_size,
                settings.embedding_local_only,
            )
        if command == "import":
            manifest = import_index(settings, store, dense)
            print(f"Index ready: movies={manifest.catalog.movie_count}, truncated={manifest.truncated_documents}")
            return
        if dense and (dense.dimension != manifest.dimension or dense.max_length != manifest.max_length):
            raise ServiceError("UNAVAILABLE", "Model and manifest mismatch")
        readiness = IndexReadiness(store, manifest)
        readiness.check()
        catalog = manifest.catalog.model_copy(
            update={"default_results": settings.default_results, "max_results": settings.max_results}
        )
        http = stack.enter_context(httpx.Client())
        llm = OpenRouterClient(
            http,
            settings.llm_base_url,
            settings.llm_api_key.get_secret_value(),
            settings.llm_model,
            settings.llm_timeout_seconds,
            settings.llm_max_output_tokens,
        )
        pipeline = RetrievalPipeline(BM25Encoder(state=manifest.bm25), dense, store, settings.rrf_k)
        service = RagService(pipeline, llm, readiness, catalog, settings.retrieval_strategy, settings.llm_context_bytes)
        from movie_rag.transport import RagHandler, create_server

        server = create_server(
            RagHandler(service, store, readiness, catalog, settings.request_timeout_seconds), readiness, settings.grpc_workers
        )
        if server.add_insecure_port(settings.grpc_addr) == 0:
            raise RuntimeError("Cannot bind gRPC address")
        server.start()
        logging.info("RAG listening on %s", settings.grpc_addr)
        try:
            server.wait_for_termination()
        except KeyboardInterrupt:
            pass
        finally:
            server.stop(grace=5).wait()


def main() -> None:
    parser = argparse.ArgumentParser(description="Movie Picker RAG")
    parser.add_argument("command", choices=["import", "serve"])
    args = parser.parse_args()
    logging.basicConfig(level=logging.INFO, format="%(levelname)s %(message)s")
    run(Settings(), args.command)


if __name__ == "__main__":
    main()
