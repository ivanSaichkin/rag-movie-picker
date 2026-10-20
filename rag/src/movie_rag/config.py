from pathlib import Path

from pydantic import Field, SecretStr, model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict

from movie_rag.models import Strategy


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", extra="ignore", allow_inf_nan=False)

    grpc_addr: str = "127.0.0.1:50051"
    grpc_workers: int = Field(default=4, ge=1, le=32)
    request_timeout_seconds: float = Field(default=60, gt=0, le=300)
    qdrant_url: str = "http://127.0.0.1:6333"
    qdrant_path: str | None = None
    qdrant_collection: str = "movies_v1"
    index_state_dir: Path = Path(".state")
    dataset_path: Path = Path("../../TMDB 5000 Movie Dataset/tmdb_5000_movies.csv")
    embedding_model: str = "intfloat/multilingual-e5-small"
    embedding_revision: str = Field(default="614241f622f53c4eeff9890bdc4f31cfecc418b3", pattern=r"^[0-9a-f]{40}$")
    embedding_device: str = "cpu"
    embedding_batch_size: int = Field(default=16, ge=1, le=256)
    embedding_local_only: bool = False
    bm25_k1: float = Field(default=1.5, gt=0)
    bm25_b: float = Field(default=0.75, ge=0, le=1)
    retrieval_strategy: Strategy = "hybrid"
    rrf_k: int = Field(default=60, gt=0)
    default_results: int = Field(default=10, ge=1)
    max_results: int = Field(default=50, ge=1, le=1000)
    llm_base_url: str = "https://openrouter.ai/api/v1"
    llm_api_key: SecretStr = SecretStr("")
    llm_model: str = ""
    llm_timeout_seconds: float = Field(default=20, gt=0)
    llm_max_output_tokens: int = Field(default=4096, ge=1)
    llm_context_bytes: int = Field(default=24000, ge=1024)

    @model_validator(mode="after")
    def validate_limits(self) -> "Settings":
        if self.default_results > self.max_results:
            raise ValueError("DEFAULT_RESULTS exceeds MAX_RESULTS")
        return self

    def index_config(self) -> dict:
        return {
            "collection": self.qdrant_collection,
            "embedding_model": self.embedding_model,
            "embedding_revision": self.embedding_revision,
            "k1": self.bm25_k1,
            "b": self.bm25_b,
            "format": "movie-text-v1",
            "tokenizer": "ascii-lower-v1",
            "normalize": True,
            "truncation": "model-max-length",
        }
