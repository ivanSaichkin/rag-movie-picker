"""Evaluate the live RAG over gRPC and save reproducible, bounded experiments."""

import argparse
import csv
import json
import random
import time
import tomllib
import uuid
from collections import defaultdict
from concurrent.futures import ThreadPoolExecutor
from datetime import UTC, datetime
from pathlib import Path
from statistics import fmean

import grpc

from movie_eval.client import RecommenderClient
from movie_eval.goldenset import (
    CatalogMovie,
    GoldenQuery,
    file_hash,
    load_catalog,
    load_goldenset,
    passes_filters,
)
from movie_eval.metrics import ranking_metrics
from movie_eval.reporting import config_snapshot, environment_snapshot, summarize

EVALUATION_DIR = Path(__file__).resolve().parent
PROJECT_DIR = EVALUATION_DIR.parent
DEFAULT_GOLDENSET = EVALUATION_DIR / "goldenset" / "queries.jsonl"
DEFAULT_CONFIG = PROJECT_DIR / "recommender" / "config.toml"


def parse_arguments(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--mode", choices=["validate", "quality", "load", "all"], default="quality")
    parser.add_argument("--config", type=Path, default=DEFAULT_CONFIG)
    parser.add_argument("--goldenset", type=Path, default=DEFAULT_GOLDENSET)
    parser.add_argument("--dataset", type=Path, help="Override the dataset_path declared in the TOML")
    parser.add_argument("--output-dir", type=Path, default=EVALUATION_DIR / "results")
    parser.add_argument("--address", help="Override server.address in the TOML")
    parser.add_argument("--strategies", nargs="+", choices=["bm25", "transformer", "hybrid"])
    parser.add_argument("--k", type=int, default=10)
    parser.add_argument("--repeats", type=int, default=1, help="Quality passes over every query")
    parser.add_argument("--warmup", type=int, default=1, help="Unmeasured requests per strategy")
    parser.add_argument("--concurrency", type=int, nargs="+", default=[1, 2, 4])
    parser.add_argument(
        "--requests",
        type=int,
        default=30,
        help="Measured requests per load level and strategy",
    )
    parser.add_argument("--timeout", type=float, default=60)
    parser.add_argument("--seed", type=int, default=42)
    parser.add_argument("--disable-llm", action="store_true", help="Disable all LLM calls, including query translation")
    parser.add_argument("--inspect", action="store_true", help="Save prompts, provider usage, raw responses and server timings")
    parser.add_argument(
        "--generate-explanations",
        action="store_true",
        help="Enable final LLM selection; can incur API costs",
    )
    parser.add_argument(
        "--allow-llm-load",
        action="store_true",
        help="Allow load mode when LLM is enabled in the local config",
    )
    args = parser.parse_args(argv)
    if args.inspect and args.mode in {"load", "all"}:
        parser.error("--inspect is for quality runs only; diagnostic serialization affects load timings")
    if args.disable_llm and args.generate_explanations:
        parser.error("--disable-llm conflicts with --generate-explanations")
    if not 1 <= args.k <= 1000 or not 1 <= args.repeats <= 100 or not 1 <= args.requests <= 10000:
        parser.error("Use k=1..1000, repeats=1..100, requests=1..10000")
    if not 0 <= args.warmup <= 100 or not 0 < args.timeout <= 300 or any(not 1 <= n <= 64 for n in args.concurrency):
        parser.error("Use warmup=0..100, timeout=(0,300], concurrency=1..64")
    if (
        len(set(args.concurrency)) != len(args.concurrency)
        or args.strategies
        and len(set(args.strategies)) != len(args.strategies)
    ):
        parser.error("Duplicate strategies or concurrency levels are not allowed")
    return args


def write_json(path: Path, value: dict) -> None:
    path.write_text(
        json.dumps(value, ensure_ascii=False, indent=2, allow_nan=False) + "\n",
        encoding="utf-8",
    )


def score_response(record: dict, case: GoldenQuery, catalog: dict[int, CatalogMovie], k: int) -> dict:
    ids = [item["movie_id"] for item in record["items"]]
    invalid = {movie_id for movie_id in ids if movie_id not in catalog or not passes_filters(catalog[movie_id], case.filters)}
    # Validate returned fields too, not just their counterparts in the CSV.
    for item in record["items"]:
        returned = CatalogMovie(
            item["movie_id"],
            item["title"],
            frozenset(item["genre_ids"]),
            item["year"],
            item["rating"],
        )
        if not passes_filters(returned, case.filters):
            invalid.add(item["movie_id"])
    judgments = {item.movie_id: item.relevance for item in case.judgments}
    metrics = ranking_metrics(ids, judgments, k, invalid_ids=frozenset(invalid))
    return {
        **record,
        "kind": case.kind,
        "expected_answer": case.expected_answer,
        "correct_rejection": (
            record.get("status") == "OK"
            and not any(w["code"] == "GENERATION_UNAVAILABLE" for w in record.get("warnings", []))
            and (
                (record.get("mode") == "empty" and not ids and any(w["code"] == "NO_MATCHES" for w in record.get("warnings", [])))
                or (
                    record.get("has_matches") is False
                    and bool(record.get("explanation"))
                    and not invalid
                    and len(ids) == len(set(ids))
                    and len(ids) <= k
                    and any(w["code"] == "NO_RELEVANT_MATCHES" for w in record.get("warnings", []))
                )
            )
        )
        if case.kind == "trap"
        else None,
        "metrics": metrics,
        "invalid_ids": sorted(invalid),
        "duplicate_count": len(ids) - len(set(ids)),
        "returned_grades": [judgments.get(movie_id, 0) for movie_id in ids],
    }


def run_stage(
    client,
    cases: list[GoldenQuery],
    catalog: dict[int, CatalogMovie],
    args,
    strategy: str,
    phase: str,
    concurrency: int,
    output,
) -> dict:
    sequence = cases * args.repeats if phase == "quality" else [cases[index % len(cases)] for index in range(args.requests)]
    random.Random(args.seed).shuffle(sequence)

    def request(index_case):
        index, case = index_case
        record = client.recommend(case, strategy, args.k, args.generate_explanations)
        record = score_response(record, case, catalog, args.k)
        return {**record, "phase": phase, "concurrency": concurrency, "sequence": index}

    started = time.perf_counter()
    records = []
    with ThreadPoolExecutor(max_workers=concurrency) as pool:
        for record in pool.map(request, enumerate(sequence)):
            records.append(record)
            output.write(json.dumps(record, ensure_ascii=False, allow_nan=False) + "\n")
            output.flush()
    wall_seconds = time.perf_counter() - started
    summary = summarize(records, wall_seconds, len(catalog))
    summary.update(phase=phase, requested_strategy=strategy, concurrency=concurrency)
    per_query = defaultdict(list)
    for record in records:
        per_query[record["query_id"]].append(record)
    summary["per_query_metrics"] = {
        query_id: {name: fmean(record["metrics"][name] for record in group) for name in group[0]["metrics"]}
        for query_id, group in per_query.items()
    }
    print(
        f"{phase:7} {strategy:11} concurrency={concurrency} OK={summary['successes']}/{summary['requests']} "
        f"NDCG@{args.k}={summary['metrics_including_failed_requests_as_zero']['ndcg_at_k']:.3f} "
        f"p95={summary['success_latency_ms']['p95']} ms",
        flush=True,
    )
    return summary


def check_server(remote: dict, args, cases: list[GoldenQuery], catalog: dict[int, CatalogMovie]) -> list[str]:
    strategies = args.strategies or remote["available_strategies"]
    if not strategies or set(strategies) - set(remote["available_strategies"]):
        raise ValueError("Requested strategies are not available in the running RAG")
    if args.k > remote["max_results"]:
        raise ValueError("Requested k exceeds the running RAG max_results")
    if remote["movie_count"] != len(catalog):
        raise ValueError("RAG movie_count differs from the local dataset; check the index/config")
    if any(set(case.filters.genre_ids) - {genre["id"] for genre in remote["genres"]} for case in cases):
        raise ValueError("Golden set contains genres unavailable in the running RAG")
    return strategies


def export_summary(path: Path, summaries: list[dict]) -> None:
    flattened = []
    for summary in summaries:
        row = {
            key: summary[key]
            for key in [
                "phase",
                "requested_strategy",
                "concurrency",
                "requests",
                "successes",
                "errors",
                "error_rate",
                "wall_seconds",
                "successful_requests_per_second",
                "degraded_requests",
                "catalog_coverage",
                "mean_fill_rate",
                "correct_rejection_rate",
                "ordinary_requests",
                "trap_requests",
            ]
        }
        row.update(summary["metrics_including_failed_requests_as_zero"])
        row.update({f"latency_{key}_ms": value for key, value in summary["success_latency_ms"].items()})
        flattened.append(row)
    if flattened:
        with path.open("w", encoding="utf-8-sig", newline="") as stream:
            writer = csv.DictWriter(stream, fieldnames=list(flattened[0]))
            writer.writeheader()
            writer.writerows(flattened)


def create_run_directory(output_root: Path) -> Path:
    output_root.mkdir(parents=True, exist_ok=True)
    existing = [int(path.name[4:]) for path in output_root.iterdir() if path.name.startswith("run_") and path.name[4:].isdigit()]
    number = max(existing, default=0) + 1
    while True:
        directory = output_root / f"run_{number}"
        try:
            directory.mkdir(exist_ok=False)
            return directory
        except FileExistsError:
            number += 1


def run_benchmark(args, client_factory=RecommenderClient) -> Path | None:
    args.config = args.config.resolve()
    with args.config.open("rb") as stream:
        config = tomllib.load(stream)
    dataset = args.dataset.resolve() if args.dataset else (args.config.parent / config["index"]["dataset_path"]).resolve()
    catalog = load_catalog(dataset)
    cases = load_goldenset(args.goldenset, catalog)
    if args.mode == "validate":
        print(f"Validated {len(cases)} queries, {sum(len(case.judgments) for case in cases)} judgments, {len(catalog)} movies.")
        return None
    if (
        args.mode in {"load", "all"}
        and config.get("llm", {}).get("enabled", True)
        and not args.allow_llm_load
        and not args.disable_llm
    ):
        raise ValueError("Load mode requires llm.enabled=false or explicit --allow-llm-load")
    directory = create_run_directory(args.output_dir.resolve())
    snapshot = config_snapshot(args.config)
    metadata = {
        "run_id": directory.name,
        "experiment_id": str(uuid.uuid4()),
        "started_utc": datetime.now(UTC).isoformat(),
        "status": "running",
        "arguments": {key: str(value) if isinstance(value, Path) else value for key, value in vars(args).items()},
        "dataset": {"path": str(dataset), "sha256": file_hash(dataset)},
        "goldenset": {
            "path": str(args.goldenset.resolve()),
            "sha256": file_hash(args.goldenset),
            "queries": len(cases),
            "judgments": sum(len(case.judgments) for case in cases),
            "status": "draft/incomplete",
        },
        "config": snapshot,
        "environment": environment_snapshot(PROJECT_DIR),
        "load_model": "closed loop, bounded client threads; not a fixed-arrival-rate capacity test",
        "latency_definition": "warm client RPC wall time, excludes executor queue wait and warmup; server-side queues included",
        "ranking_definition": (
            "unjudged/invalid IDs and repeated IDs gain zero; AP@k denominator=min(number of grade>=2 labels,k)"
        ),
    }
    write_json(directory / "metadata.json", metadata)
    (directory / "goldenset.jsonl").write_bytes(args.goldenset.read_bytes())
    client = client_factory(
        args.address or config.get("server", {}).get("address", "127.0.0.1:50051"),
        args.timeout,
    )
    client.disable_llm = args.disable_llm
    client.inspect = args.inspect
    summaries = []
    try:
        remote = client.catalog()
        metadata["server_catalog"] = remote
        strategies = check_server(remote, args, cases, catalog)
        with (
            (directory / "requests.jsonl").open("w", encoding="utf-8") as output,
            (directory / "warmup.jsonl").open("w", encoding="utf-8") as warmup,
        ):
            for strategy in strategies:
                for index in range(args.warmup):
                    record = client.recommend(
                        cases[index % len(cases)],
                        strategy,
                        args.k,
                        args.generate_explanations,
                    )
                    warmup.write(json.dumps(record, ensure_ascii=False) + "\n")
                if args.mode in {"quality", "all"}:
                    summaries.append(run_stage(client, cases, catalog, args, strategy, "quality", 1, output))
                if args.mode in {"load", "all"}:
                    for concurrency in args.concurrency:
                        summaries.append(
                            run_stage(
                                client,
                                cases,
                                catalog,
                                args,
                                strategy,
                                "load",
                                concurrency,
                                output,
                            )
                        )
        metadata["status"] = "completed"
        if any(summary["errors"] or summary["degraded_requests"] for summary in summaries):
            metadata["status"] = "completed_with_errors_or_degradation"
    except BaseException as error:
        metadata["status"] = "interrupted" if isinstance(error, KeyboardInterrupt) else "failed"
        metadata["error"] = {"type": type(error).__name__, "message": str(error)}
        raise
    finally:
        client.close()
        metadata["finished_utc"] = datetime.now(UTC).isoformat()
        write_json(directory / "metadata.json", metadata)
        write_json(directory / "summary.json", {"stages": summaries})
        export_summary(directory / "summary.csv", summaries)
        requests_path = directory / "requests.jsonl"
        if requests_path.exists():
            with (directory / "answer_review.jsonl").open("w", encoding="utf-8") as review:
                for line in requests_path.read_text(encoding="utf-8").splitlines():
                    record = json.loads(line)
                    if record["phase"] == "quality" and record.get("kind", "normal") == "normal":
                        review.write(
                            json.dumps(
                                {
                                    "query_id": record["query_id"],
                                    "strategy": record["requested_strategy"],
                                    "sequence": record["sequence"],
                                    "answer_correct": None,
                                    "comment": "",
                                },
                                ensure_ascii=False,
                            )
                            + "\n"
                        )
        print(f"Results: {directory}", flush=True)
    return directory


def main() -> None:
    try:
        run_benchmark(parse_arguments())
    except (ValueError, OSError, grpc.RpcError) as error:
        raise SystemExit(f"Benchmark failed: {error}") from error


if __name__ == "__main__":
    main()
