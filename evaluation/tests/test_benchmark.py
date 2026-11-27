import csv
import json
import math
import tempfile
import unittest
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

import grpc

from benchmark import (
    DEFAULT_CONFIG,
    DEFAULT_GOLDENSET,
    create_run_directory,
    parse_arguments,
    run_benchmark,
    score_response,
)
from movie_eval.client import pb, rpc
from movie_eval.goldenset import load_catalog, load_goldenset
from movie_eval.metrics import percentile, ranking_metrics
from movie_eval.reporting import config_snapshot, summarize


class MetricsTests(unittest.TestCase):
    def test_perfect_ranking(self):
        result = ranking_metrics([1, 2], {1: 3, 2: 2}, 2)
        self.assertTrue(all(value == 1 for value in result.values()))

    def test_manual_ranking_with_duplicate_and_unjudged(self):
        result = ranking_metrics([3, 99, 1, 1, 2], {1: 3, 2: 2, 3: 0}, 5)
        self.assertAlmostEqual(result["ap_at_k"], (1 / 3 + 2 / 5) / 2)
        self.assertAlmostEqual(
            result["ndcg_at_k"],
            (7 / math.log2(4) + 3 / math.log2(6)) / (7 + 3 / math.log2(3)),
        )
        self.assertEqual(result["precision_at_k"], 2 / 5)
        self.assertEqual(result["recall_at_k"], 1)
        self.assertEqual(result["reciprocal_rank_at_k"], 1 / 3)

    def test_short_empty_and_invalid_rankings(self):
        self.assertEqual(ranking_metrics([1], {1: 3}, 10)["precision_at_k"], 0.1)
        self.assertTrue(all(value == 0 for value in ranking_metrics([], {1: 3}, 10).values()))
        self.assertEqual(ranking_metrics([1], {1: 3}, 1, invalid_ids=frozenset({1}))["ndcg_at_k"], 0)

    def test_percentile_interpolation(self):
        self.assertEqual(percentile([4, 1, 3, 2], 50), 2.5)
        self.assertEqual(percentile([4], 99), 4)
        self.assertIsNone(percentile([], 99))


class GoldenSetTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        # Synthetic cards exercise the evaluator without distributing the TMDB CSV.
        cls.temporary = tempfile.TemporaryDirectory()
        cls.addClassCleanup(cls.temporary.cleanup)
        cls.dataset = Path(cls.temporary.name) / "movies.csv"
        labels = [json.loads(line) for line in DEFAULT_GOLDENSET.read_text(encoding="utf-8").splitlines()]
        movies = {movie["movie_id"]: movie for case in labels for movie in case["relevant_movies"]}
        with cls.dataset.open("w", encoding="utf-8", newline="") as stream:
            writer = csv.DictWriter(
                stream, fieldnames=["id", "title", "overview", "genres", "vote_average", "vote_count", "release_date"]
            )
            writer.writeheader()
            for movie in movies.values():
                writer.writerow(
                    {
                        "id": movie["movie_id"],
                        "title": movie["title"],
                        "overview": "Synthetic test card",
                        "genres": '[{"id":16}]',
                        "vote_average": "7.5",
                        "vote_count": "100",
                        "release_date": "2010-01-01",
                    }
                )
        cls.catalog = load_catalog(cls.dataset)
        cls.cases = load_goldenset(DEFAULT_GOLDENSET, cls.catalog)

    def test_golden_set_shape(self):
        ordinary = [case for case in self.cases if case.kind == "normal"]
        traps = [case for case in self.cases if case.kind == "trap"]
        self.assertEqual(len(ordinary), 10)
        self.assertEqual(len(traps), 5)
        self.assertTrue(all(10 <= len(case.judgments) <= 20 for case in ordinary))
        self.assertTrue(all(not case.judgments and case.expected_answer for case in traps))

    def test_unknown_ids_and_wrong_titles_rejected(self):
        raw = json.loads(DEFAULT_GOLDENSET.read_text(encoding="utf-8").splitlines()[0])
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / "invalid.jsonl"
            raw["relevant_movies"][0]["title"] = "wrong title"
            path.write_text(json.dumps(raw), encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "not found"):
                load_goldenset(path, self.catalog)

    def test_returned_filter_violation_scores_zero(self):
        case = next(case for case in self.cases if case.filters.genre_ids)
        judgment = case.judgments[0]
        record = {
            "items": [
                {
                    "movie_id": judgment.movie_id,
                    "title": judgment.title,
                    "genre_ids": [],
                    "year": None,
                    "rating": None,
                }
            ]
        }
        result = score_response(record, case, self.catalog, 10)
        self.assertEqual(result["invalid_ids"], [judgment.movie_id])
        self.assertEqual(result["metrics"]["ndcg_at_k"], 0)

    def test_errors_are_not_omitted_from_metrics(self):
        case = self.cases[0]
        base = {
            "status": "UNAVAILABLE",
            "error": "test",
            "mode": None,
            "strategy_used": None,
            "requested_strategy": "bm25",
            "requested_k": 10,
            "items": [],
            "warnings": [],
            "latency_ms": 10,
        }
        record = score_response(base, case, self.catalog, 10)
        summary = summarize([record], 0.1, len(self.catalog))
        self.assertEqual(summary["error_rate"], 1)
        self.assertIsNone(summary["success_latency_ms"]["p95"])
        self.assertEqual(summary["metrics_including_failed_requests_as_zero"]["ndcg_at_k"], 0)

    def test_run_directories_never_overwrite(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            self.assertEqual(create_run_directory(root).name, "run_1")
            self.assertEqual(create_run_directory(root).name, "run_2")

    def test_config_snapshot_excludes_secrets(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / "config.toml"
            path.write_text('[llm]\napi_key="do-not-save-this"\nenabled=false\n', encoding="utf-8")
            self.assertNotIn("do-not-save-this", json.dumps(config_snapshot(path)))

    def test_complete_benchmark_against_test_grpc_server(self):
        catalog, cases = self.catalog, self.cases

        class Fixture(rpc.RagServiceServicer):
            def GetCatalog(self, request, context):
                return pb.Catalog(
                    movie_count=len(catalog),
                    max_results=50,
                    available_strategies=["bm25", "transformer", "hybrid"],
                    genres=[pb.Genre(id=16, name="Animation")],
                )

            def Recommend(self, request, context):
                case = next(case for case in cases if case.query == request.query)
                items = []
                for judgment in case.judgments[: request.limit]:
                    movie = catalog[judgment.movie_id]
                    message = pb.Movie(
                        movie_id=movie.movie_id,
                        title=movie.title,
                        genres=[pb.Genre(id=genre, name=str(genre)) for genre in movie.genre_ids],
                    )
                    if movie.year is not None:
                        message.year = movie.year
                    if movie.rating is not None:
                        message.rating = movie.rating
                    items.append(pb.Recommendation(movie=message))
                return pb.RecommendResponse(mode="retrieval_only", strategy_used=request.strategy, items=items)

        server = grpc.server(ThreadPoolExecutor(max_workers=4))
        rpc.add_RagServiceServicer_to_server(Fixture(), server)
        port = server.add_insecure_port("127.0.0.1:0")
        server.start()
        try:
            with tempfile.TemporaryDirectory() as temporary:
                args = parse_arguments(
                    [
                        "--mode",
                        "all",
                        "--address",
                        f"127.0.0.1:{port}",
                        "--config",
                        str(DEFAULT_CONFIG),
                        "--dataset",
                        str(self.dataset),
                        "--output-dir",
                        temporary,
                        "--k",
                        "5",
                        "--requests",
                        "6",
                        "--concurrency",
                        "1",
                        "2",
                        "--allow-llm-load",
                    ]
                )
                directory = run_benchmark(args)
                metadata = json.loads((directory / "metadata.json").read_text(encoding="utf-8"))
                summary = json.loads((directory / "summary.json").read_text(encoding="utf-8"))
                self.assertEqual(metadata["status"], "completed")
                self.assertEqual(len(summary["stages"]), 9)
                self.assertTrue(all(stage["errors"] == 0 for stage in summary["stages"]))
                self.assertTrue(
                    all(stage["metrics_including_failed_requests_as_zero"]["ndcg_at_k"] == 1 for stage in summary["stages"])
                )
                records = (directory / "requests.jsonl").read_text(encoding="utf-8").splitlines()
                self.assertEqual(len(records), 81)
                self.assertTrue((directory / "summary.csv").exists())
        finally:
            server.stop(0).wait()


if __name__ == "__main__":
    unittest.main()
