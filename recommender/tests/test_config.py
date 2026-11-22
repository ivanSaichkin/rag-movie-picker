from pathlib import Path

import pytest

from movie_rag.config import AppConfig, load_config


def test_toml_paths_are_relative_to_config(tmp_path, monkeypatch):
    path = tmp_path / "config.toml"
    path.write_text(
        '[index]\nenabled_vectors = ["bm25"]\ndataset_path = "movies.csv"\n'
        '[search]\ndefault_strategy = "bm25"\n[llm]\nenabled = false\n',
        encoding="utf-8",
    )
    monkeypatch.setenv("MAX_RESULTS", "999")
    config = load_config(path)
    assert config.index.dataset_path == tmp_path / "movies.csv"
    assert config.index.state_dir == tmp_path / ".state"
    assert config.available_strategies == ["bm25"]
    assert config.search.max_results == 50


@pytest.mark.parametrize("vectors,strategy", [([], "bm25"), (["bm25", "bm25"], "bm25"), (["bm25"], "hybrid")])
def test_invalid_vector_configuration(vectors, strategy):
    with pytest.raises(ValueError):
        AppConfig(index={"enabled_vectors": vectors}, search={"default_strategy": strategy})


def test_unused_encoder_settings_do_not_change_index_identity():
    config = AppConfig(index={"enabled_vectors": ["bm25"]}, search={"default_strategy": "bm25"})
    changed = config.model_copy(deep=True)
    changed.transformer.model = "another-model"
    changed.search.min_candidates = 100
    assert config.index_config() == changed.index_config()


def test_invalid_limits_and_unknown_config_keys():
    with pytest.raises(ValueError):
        AppConfig(search={"default_results": 20, "max_results": 10})
    with pytest.raises(ValueError):
        AppConfig(search={"defaut_strategy": "bm25"})


@pytest.mark.parametrize("name", ["config.toml", "config.bm25.toml"])
def test_shipped_configuration_is_valid(name):
    path = Path(__file__).parents[1] / name
    config = load_config(path)
    assert config.search.default_strategy in config.available_strategies
    assert config.index.dataset_path.is_absolute()
