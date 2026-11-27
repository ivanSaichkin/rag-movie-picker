import math

import pytest
from conftest import FakeDense
from pydantic import ValidationError

from movie_rag.embeddings import BM25Encoder, BM25State, HybridEmbedder
from movie_rag.models import SparseVector


def test_bm25_manual_calculation():
    encoder = BM25Encoder()
    encoder.fit(["cat cat dog", "dog bird"])
    query = encoder.encode_query("CAT cat unknown").sparse
    document = encoder.encode_documents(["cat cat dog"])[0].sparse
    expected_idf = math.log(1 + (2 - 1 + 0.5) / (1 + 0.5))
    expected_tf = 2 * 2.5 / (2 + 1.5 * (1 - 0.75 + 0.75 * 3 / 2.5))
    doc = dict(zip(document.indices, document.values, strict=True))
    score = sum(doc[i] * v for i, v in zip(query.indices, query.values, strict=True))
    assert score == pytest.approx(expected_idf * expected_tf)
    assert query == encoder.encode_query("cat").sparse
    assert encoder.encode_query("none").sparse.indices == []


def test_state_roundtrip_and_deterministic_vocabulary():
    encoder = BM25Encoder()
    encoder.fit(["dog cat cat", "bird"])
    restored = BM25Encoder(state=BM25State.model_validate_json(encoder.state.model_dump_json()))
    assert restored.encode_query("dog bird") == encoder.encode_query("dog bird")
    assert encoder.state.vocabulary == ["bird", "cat", "dog"]
    assert encoder.state.df == [1, 1, 1]


@pytest.mark.parametrize("texts", [[], ["!!!", ""]])
def test_empty_corpus(texts):
    with pytest.raises(ValueError):
        BM25Encoder().fit(texts)


def test_hybrid_batch():
    sparse = BM25Encoder()
    sparse.fit(["space", "garden"])
    output = HybridEmbedder(sparse, FakeDense()).encode_documents(["space", "garden"])
    assert len(output) == 2
    assert all(item.sparse is not None and item.dense is not None for item in output)


def test_sparse_validation():
    with pytest.raises(ValidationError):
        SparseVector(indices=[1, 1], values=[1.0, 2.0])


def test_bilingual_stemming_roundtrip_and_document_frequency():
    encoder = BM25Encoder(tokenizer="snowball-ru-en-v1")
    encoder.fit(["Фильмы о космонавтах movies", "фильм о космонавте movie", "ёлка"])
    assert encoder.tokenize("ФИЛЬМЫ MOVIES ёлки") == encoder.tokenize("фильм movie елка")
    assert encoder.state.df[encoder.state.vocabulary.index(encoder.tokenize("космонавт")[0])] == 2
    restored = BM25Encoder(state=BM25State.model_validate_json(encoder.state.model_dump_json()))
    assert restored.encode_query("космонавтом movies") == encoder.encode_query("космонавты movie")
    assert restored.encode_query("космонавт").sparse.indices


def test_legacy_tokenization_unchanged():
    assert BM25Encoder().tokenize("MOVIES фильм 2001") == ["movies", "2001"]
