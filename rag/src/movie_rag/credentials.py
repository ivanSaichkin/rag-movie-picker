import os
from pathlib import Path

from dotenv import dotenv_values


def read_api_key(variable_name: str, env_file: Path | None) -> str:
    """Process environment wins; the file contributes only the selected secret."""
    if variable_name in os.environ:
        return os.environ[variable_name]
    if env_file is None or not env_file.is_file():
        return ""
    values = dotenv_values(env_file, encoding="utf-8-sig", interpolate=False)
    return values.get(variable_name) or ""
