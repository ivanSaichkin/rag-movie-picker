import tomllib
from pathlib import Path
from typing import Literal

from pydantic import Field, model_validator

from movie_rag.models import Model, Strategy, VectorKind, available_strategies


class IndexConfig(Model):
    enabled_vectors: list[VectorKind] = Field(default_factory=lambda: ["bm25", "transformer"], min_length=1)
    state_dir: Path = Path(".state")
    dataset_path: Path = Path("../../data/raw/TMDB 5000 Movie Dataset/tmdb_5000_movies.csv")
    batch_size: int = Field(default=16, ge=1, le=256)

    @model_validator(mode="after")
    def unique_vectors(self) -> "IndexConfig":
        if len(set(self.enabled_vectors)) != len(self.enabled_vectors):
            raise ValueError("enabled_vectors must be unique")
        return self


class SearchConfig(Model):
    default_strategy: Strategy = "hybrid"
    default_results: int = Field(default=10, ge=1)
    max_results: int = Field(default=50, ge=1, le=1000)
    candidate_multiplier: int = Field(default=3, ge=1)
    min_candidates: int = Field(default=30, ge=1)
    max_candidates: int = Field(default=1000, ge=1, le=1000)
    rrf_k: int = Field(default=60, gt=0)
    generate_explanations: bool = True

    @model_validator(mode="after")
    def valid_limits(self) -> "SearchConfig":
        if self.default_results > self.max_results:
            raise ValueError("default_results exceeds max_results")
        if max(self.max_results, self.min_candidates) > self.max_candidates:
            raise ValueError("max_candidates must cover max_results and min_candidates")
        return self


class BM25Config(Model):
    k1: float = Field(default=1.5, gt=0)
    b: float = Field(default=0.75, ge=0, le=1)
    query_preparation: Literal["llm", "none"] = "llm"


class TransformerConfig(Model):
    model: str = "intfloat/multilingual-e5-small"
    revision: str = Field(default="614241f622f53c4eeff9890bdc4f31cfecc418b3", pattern=r"^[0-9a-f]{40}$")
    device: str = "cpu"
    batch_size: int = Field(default=16, ge=1, le=256)
    local_only: bool = False


class QdrantConfig(Model):
    url: str = "http://127.0.0.1:6333"
    path: Path | None = None
    collection: str = "movies_v1"


class LLMConfig(Model):
    enabled: bool = True
    base_url: str = "https://openrouter.ai/api/v1"
    model: str = ""
    api_key_env: str = "LLM_API_KEY"
    timeout_seconds: float = Field(default=20, gt=0)
    max_output_tokens: int = Field(default=4096, ge=1)
    context_bytes: int = Field(default=24000, ge=1024)


class ServerConfig(Model):
    address: str = "127.0.0.1:50051"
    workers: int = Field(default=4, ge=1, le=32)
    request_timeout_seconds: float = Field(default=60, gt=0, le=300)


class AppConfig(Model):
    index: IndexConfig = Field(default_factory=IndexConfig)
    search: SearchConfig = Field(default_factory=SearchConfig)
    bm25: BM25Config = Field(default_factory=BM25Config)
    transformer: TransformerConfig = Field(default_factory=TransformerConfig)
    qdrant: QdrantConfig = Field(default_factory=QdrantConfig)
    llm: LLMConfig = Field(default_factory=LLMConfig)
    server: ServerConfig = Field(default_factory=ServerConfig)

    @property
    def available_strategies(self) -> list[Strategy]:
        return available_strategies(self.index.enabled_vectors)

    @model_validator(mode="after")
    def supported_default_strategy(self) -> "AppConfig":
        if self.search.default_strategy not in self.available_strategies:
            raise ValueError("default_strategy requires vectors not enabled in the index")
        return self

    def index_config(self) -> dict:
        config = {"collection": self.qdrant.collection, "vectors": sorted(self.index.enabled_vectors), "format": "movie-text-v1"}
        if "bm25" in self.index.enabled_vectors:
            config["bm25"] = {"k1": self.bm25.k1, "b": self.bm25.b, "tokenizer": "ascii-lower-v1"}
        if "transformer" in self.index.enabled_vectors:
            config["transformer"] = {
                "model": self.transformer.model,
                "revision": self.transformer.revision,
                "normalize": True,
                "truncation": "model-max-length",
            }
        return config


def load_config(path: Path) -> AppConfig:
    path = path.resolve()
    with path.open("rb") as stream:
        config = AppConfig.model_validate(tomllib.load(stream))
    config.index.dataset_path = (path.parent / config.index.dataset_path).resolve()
    config.index.state_dir = (path.parent / config.index.state_dir).resolve()
    if config.qdrant.path is not None:
        config.qdrant.path = (path.parent / config.qdrant.path).resolve()
    return config
