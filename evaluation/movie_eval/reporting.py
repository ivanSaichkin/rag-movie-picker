import importlib.metadata
import json
import platform
import subprocess
import tomllib
from collections import Counter
from collections.abc import Sequence
from pathlib import Path
from statistics import fmean

from movie_eval.goldenset import file_hash
from movie_eval.metrics import percentile

CONFIG_FIELDS = {
    "index": ["enabled_vectors", "state_dir", "dataset_path", "batch_size"],
    "search": [
        "default_strategy",
        "default_results",
        "max_results",
        "candidate_multiplier",
        "min_candidates",
        "max_candidates",
        "rrf_k",
        "generate_explanations",
    ],
    "bm25": ["k1", "b", "query_preparation", "tokenizer"],
    "transformer": ["model", "revision", "device", "batch_size", "local_only"],
    "qdrant": ["url", "path", "collection"],
    "llm": [
        "enabled",
        "base_url",
        "model",
        "timeout_seconds",
        "max_output_tokens",
        "context_bytes",
        "reasoning_enabled",
        "structured_outputs",
    ],
    "server": ["address", "workers", "request_timeout_seconds"],
}


def config_snapshot(path: Path) -> dict:
    with path.open("rb") as stream:
        raw = tomllib.load(stream)
    filtered = {
        section: {key: raw.get(section, {})[key] for key in keys if key in raw.get(section, {})}
        for section, keys in CONFIG_FIELDS.items()
    }
    state = (path.parent / raw.get("index", {}).get("state_dir", ".state")).resolve()
    manifest_path = state / "manifest.json"
    manifest = None
    if manifest_path.exists():
        source = json.loads(manifest_path.read_text(encoding="utf-8"))
        manifest = {
            key: source.get(key)
            for key in [
                "version",
                "status",
                "index_id",
                "csv_sha256",
                "config",
                "vector_schema",
                "max_length",
            ]
        }
    return {
        "source": str(path.resolve()),
        "source_sha256": file_hash(path),
        "declared_config": filtered,
        "local_manifest": manifest,
        "server_config_attested": False,
        "note": "Snapshot of a local file, not proof of the running server configuration; restart RAG with this file.",
    }


def environment_snapshot(repo: Path) -> dict:
    def git(*arguments):
        result = subprocess.run(
            ["git", "-C", str(repo), *arguments],
            check=False,
            capture_output=True,
            text=True,
            timeout=10,
        )
        return result.stdout.strip() if result.returncode == 0 else None

    versions = {name: importlib.metadata.version(name) for name in ["grpcio", "protobuf"]}
    return {
        "python": platform.python_version(),
        "platform": platform.platform(),
        "client_packages": versions,
        "rag_commit": git("rev-parse", "HEAD"),
        "rag_dirty": bool(git("status", "--porcelain")),
        "proto_sha256": file_hash(repo / "proto" / "rag" / "v1" / "rag.proto"),
    }


def summarize(records: Sequence[dict], wall_seconds: float, catalog_count: int) -> dict:
    ordinary = [record for record in records if record.get("kind", "normal") == "normal"]
    traps = [record for record in records if record.get("kind") == "trap"]
    successful = [record for record in records if record["status"] == "OK"]
    latency = [record["latency_ms"] for record in successful]
    all_latency = [record["latency_ms"] for record in records]
    metric_names = [
        "ndcg_at_k",
        "ap_at_k",
        "precision_at_k",
        "recall_at_k",
        "reciprocal_rank_at_k",
        "hit_at_k",
    ]
    metric_labels = {
        "ap_at_k": "map_at_k",
        "reciprocal_rank_at_k": "mrr_at_k",
        "hit_at_k": "hit_rate_at_k",
    }
    metrics = {
        metric_labels.get(name, name): fmean(record["metrics"][name] for record in ordinary) if ordinary else 0.0
        for name in metric_names
    }
    unique_ids = {
        item["movie_id"] for record in successful for item in record["items"] if item["movie_id"] not in record["invalid_ids"]
    }
    return {
        "ordinary_requests": len(ordinary),
        "trap_requests": len(traps),
        "correct_rejection_rate": fmean(record["correct_rejection"] for record in traps) if traps else None,
        "ranking_population": "ordinary queries only; failed ordinary requests count as zero",
        "requests": len(records),
        "successes": len(successful),
        "errors": len(records) - len(successful),
        "error_rate": (len(records) - len(successful)) / len(records) if records else 0.0,
        "wall_seconds": wall_seconds,
        "completed_requests_per_second": len(records) / wall_seconds if wall_seconds else 0.0,
        "successful_requests_per_second": len(successful) / wall_seconds if wall_seconds else 0.0,
        "success_latency_ms": {
            "mean": fmean(latency) if latency else None,
            "p50": percentile(latency, 50),
            "p95": percentile(latency, 95),
            "p99": percentile(latency, 99),
            "max": max(latency) if latency else None,
        },
        "all_attempt_latency_ms": {
            "p50": percentile(all_latency, 50),
            "p95": percentile(all_latency, 95),
            "p99": percentile(all_latency, 99),
        },
        "status_counts": dict(Counter(record["status"] for record in records)),
        "strategy_used_counts": dict(Counter(record["strategy_used"] for record in successful)),
        "mode_counts": dict(Counter(record["mode"] for record in successful)),
        "warning_counts": dict(Counter(warning["code"] for record in records for warning in record["warnings"])),
        "degraded_requests": sum(
            record["status"] == "OK" and record["strategy_used"] != record["requested_strategy"] for record in records
        ),
        "invalid_filter_or_unknown_id_count": sum(len(record["invalid_ids"]) for record in records),
        "duplicate_result_count": sum(record["duplicate_count"] for record in records),
        "over_limit_count": sum(len(record["items"]) > record["requested_k"] for record in records),
        "mean_fill_rate": fmean(min(len(record["items"]), record["requested_k"]) / record["requested_k"] for record in records)
        if records
        else 0.0,
        "catalog_coverage": len(unique_ids) / catalog_count if catalog_count else 0.0,
        "metrics_including_failed_requests_as_zero": metrics,
    }
