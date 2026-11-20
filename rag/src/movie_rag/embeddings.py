import math
import re
import threading
from collections import Counter

from pydantic import Field, model_validator

from movie_rag.models import Model, PreparedQuery, ServiceError, SparseVector, VectorRepresentation
from movie_rag.ports import EmbeddingModel


class BM25State(Model):
    vocabulary: list[str]
    df: list[int]
    n: int = Field(gt=0)
    avgdl: float = Field(gt=0)
    k1: float = Field(default=1.5, gt=0)
    b: float = Field(default=0.75, ge=0, le=1)

    @model_validator(mode="after")
    def check_statistics(self) -> "BM25State":
        if self.vocabulary != sorted(set(self.vocabulary)) or len(self.vocabulary) != len(self.df):
            raise ValueError("Invalid BM25 vocabulary")
        if not self.vocabulary or any(df < 1 or df > self.n for df in self.df):
            raise ValueError("Invalid document frequencies")
        return self


class BM25Encoder:
    def __init__(self, k1: float = 1.5, b: float = 0.75, state: BM25State | None = None) -> None:
        if not math.isfinite(k1) or not math.isfinite(b) or k1 <= 0 or not 0 <= b <= 1:
            raise ValueError("Invalid BM25 parameters")
        self.k1, self.b, self.state = k1, b, state
        self._ids = {term: i for i, term in enumerate(state.vocabulary)} if state else {}
        if state:
            self.k1, self.b = state.k1, state.b

    @staticmethod
    def tokenize(text: str) -> list[str]:
        return re.findall(r"[a-z0-9]+", text.lower())

    def fit(self, texts: list[str]) -> None:
        documents = [self.tokenize(text) for text in texts]
        length = sum(map(len, documents))
        if not documents or not length:
            raise ValueError("Empty BM25 corpus")
        df = Counter(term for tokens in documents for term in set(tokens))
        vocabulary = sorted(df)
        self.state = BM25State(
            vocabulary=vocabulary,
            df=[df[t] for t in vocabulary],
            n=len(documents),
            avgdl=length / len(documents),
            k1=self.k1,
            b=self.b,
        )
        self._ids = {term: i for i, term in enumerate(vocabulary)}

    def _require_state(self) -> BM25State:
        if self.state is None:
            raise ValueError("BM25 must be fitted before encoding")
        return self.state

    def encode_documents(self, texts: list[str]) -> list[VectorRepresentation]:
        state = self._require_state()
        result = []
        for text in texts:
            tokens = self.tokenize(text)
            frequencies = Counter(tokens)
            indices, values = [], []
            for term in sorted(frequencies):
                if term not in self._ids:
                    continue
                tf = frequencies[term]
                value = tf * (self.k1 + 1) / (tf + self.k1 * (1 - self.b + self.b * len(tokens) / state.avgdl))
                indices.append(self._ids[term])
                values.append(value)
            result.append(VectorRepresentation(sparse=SparseVector(indices=indices, values=values)))
        return result

    def encode_query(self, text: str) -> VectorRepresentation:
        state = self._require_state()
        indices = sorted({self._ids[t] for t in self.tokenize(text) if t in self._ids})
        values = [math.log(1 + (state.n - state.df[i] + 0.5) / (state.df[i] + 0.5)) for i in indices]
        return VectorRepresentation(sparse=SparseVector(indices=indices, values=values))


class TransformerEmbedder:
    def __init__(self, name: str, revision: str, device: str = "cpu", batch_size: int = 16, local_only: bool = False) -> None:
        from sentence_transformers import SentenceTransformer

        self.model = SentenceTransformer(
            name, revision=revision, device=device, local_files_only=local_only, trust_remote_code=False
        )
        self.batch_size = batch_size
        self.dimension = self.model.get_embedding_dimension()
        self.max_length = self.model.max_seq_length
        self.truncated_documents = 0
        self._lock = threading.Lock()

    def _encode(self, texts: list[str], prefix: str) -> list[VectorRepresentation]:
        if not texts:
            return []
        with self._lock:
            prefixed = [prefix + text for text in texts]
            if prefix == "passage: ":
                lengths = self.model.tokenizer(prefixed, truncation=False, padding=False, verbose=False)["input_ids"]
                self.truncated_documents += sum(len(tokens) > self.max_length for tokens in lengths)
            vectors = self.model.encode(
                prefixed,
                batch_size=self.batch_size,
                normalize_embeddings=True,
                show_progress_bar=False,
                convert_to_numpy=True,
            )
            return [VectorRepresentation(dense=vector.tolist()) for vector in vectors]

    def encode_documents(self, texts: list[str]) -> list[VectorRepresentation]:
        return self._encode(texts, "passage: ")

    def encode_query(self, text: str) -> VectorRepresentation:
        try:
            return self._encode([text], "query: ")[0]
        except (RuntimeError, OSError) as exc:
            raise ServiceError("UNAVAILABLE", "Transformer inference unavailable") from exc


class HybridEmbedder:
    def __init__(self, sparse: EmbeddingModel, dense: EmbeddingModel) -> None:
        self.sparse, self.dense = sparse, dense

    def encode_documents(self, texts: list[str]) -> list[VectorRepresentation]:
        sparse, dense = self.sparse.encode_documents(texts), self.dense.encode_documents(texts)
        if len(sparse) != len(texts) or len(dense) != len(texts):
            raise ValueError("Encoder batch length mismatch")
        return [VectorRepresentation(sparse=s.sparse, dense=d.dense) for s, d in zip(sparse, dense, strict=True)]

    def encode_query(self, query: PreparedQuery) -> VectorRepresentation:
        sparse = self.sparse.encode_query(query.sparse_en).sparse if query.sparse_en is not None else None
        return VectorRepresentation(sparse=sparse, dense=self.dense.encode_query(query.original).dense)
