import pytest

from movie_rag.credentials import read_api_key


def test_reads_quoted_key_from_env_file(tmp_path, monkeypatch):
    monkeypatch.delenv("LLM_API_KEY", raising=False)
    path = tmp_path / ".env"
    path.write_text('# OpenRouter\nLLM_API_KEY="test-value"\n', encoding="utf-8-sig")
    assert read_api_key("LLM_API_KEY", path) == "test-value"


@pytest.mark.parametrize("value", ["process-value", ""])
def test_process_environment_takes_priority_even_when_empty(tmp_path, monkeypatch, value):
    monkeypatch.setenv("LLM_API_KEY", value)
    path = tmp_path / ".env"
    path.write_text("LLM_API_KEY=file-value\n", encoding="utf-8")
    assert read_api_key("LLM_API_KEY", path) == value


def test_missing_file_and_key_return_empty(tmp_path, monkeypatch):
    monkeypatch.delenv("LLM_API_KEY", raising=False)
    assert read_api_key("LLM_API_KEY", tmp_path / "missing.env") == ""


def test_only_selected_key_is_read_without_mutating_environment(tmp_path, monkeypatch):
    monkeypatch.delenv("CUSTOM_KEY", raising=False)
    monkeypatch.delenv("MAX_RESULTS", raising=False)
    path = tmp_path / ".env"
    path.write_text("CUSTOM_KEY=secret\nMAX_RESULTS=999\n", encoding="utf-8")
    assert read_api_key("CUSTOM_KEY", path) == "secret"
    import os

    assert "CUSTOM_KEY" not in os.environ
    assert "MAX_RESULTS" not in os.environ
