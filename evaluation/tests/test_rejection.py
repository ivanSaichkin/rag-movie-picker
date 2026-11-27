import json
import tempfile
import unittest
from pathlib import Path

from benchmark import score_response
from grade_answers import grade
from movie_eval.goldenset import CatalogMovie, GoldenQuery, MovieFilters
from movie_eval.reporting import summarize


class RejectionTests(unittest.TestCase):
    def test_errors_are_not_rejections_and_traps_do_not_dilute_ranking(self):
        case = GoldenQuery("absent", "missing", "", MovieFilters(), (), "trap", "No matches")
        base = dict(
            status="OK",
            mode="empty",
            items=[],
            warnings=[{"code": "NO_MATCHES"}],
            latency_ms=1,
            strategy_used="bm25",
            requested_strategy="bm25",
            requested_k=10,
        )
        good = score_response(base, case, {}, 10)
        error = score_response({**base, "status": "UNAVAILABLE"}, case, {}, 10)
        ordinary = {**good, "kind": "normal", "metrics": {name: 1.0 for name in good["metrics"]}}
        summary = summarize([good, error, ordinary], 1, 1)
        self.assertEqual(summary["correct_rejection_rate"], 0.5)
        self.assertEqual(summary["metrics_including_failed_requests_as_zero"]["ndcg_at_k"], 1)

    def test_accuracy_requires_complete_manual_review(self):
        with tempfile.TemporaryDirectory() as folder:
            directory = Path(folder)
            request = dict(query_id="q", requested_strategy="bm25", sequence=0, phase="quality", status="OK")
            review = dict(query_id="q", strategy="bm25", sequence=0, answer_correct=None)
            (directory / "requests.jsonl").write_text(json.dumps(request), encoding="utf-8")
            (directory / "answer_review.jsonl").write_text(json.dumps(review), encoding="utf-8")
            self.assertIsNone(grade(directory)["bm25"]["accuracy"])
            review["answer_correct"] = True
            (directory / "answer_review.jsonl").write_text(json.dumps(review), encoding="utf-8")
            self.assertEqual(grade(directory)["bm25"]["accuracy"], 1)
            request["status"] = "UNAVAILABLE"
            (directory / "requests.jsonl").write_text(json.dumps(request), encoding="utf-8")
            self.assertEqual(grade(directory)["bm25"]["accuracy"], 0)

    def test_alternatives_count_as_rejection_only_with_explicit_status(self):
        case = GoldenQuery("trap", "missing", "", MovieFilters(), (), "trap", "Explain absence")
        catalog = {1: CatalogMovie(1, "Alternative", frozenset(), None, None)}
        record = dict(
            status="OK",
            mode="generated",
            has_matches=False,
            explanation="No matches; here is an alternative",
            warnings=[{"code": "NO_RELEVANT_MATCHES"}],
            items=[dict(movie_id=1, title="Alternative", genre_ids=[], year=None, rating=None)],
        )
        self.assertTrue(score_response(record, case, catalog, 1)["correct_rejection"])
        self.assertFalse(score_response({**record, "has_matches": True}, case, catalog, 1)["correct_rejection"])
        failed = {**record, "warnings": [*record["warnings"], {"code": "GENERATION_UNAVAILABLE"}]}
        self.assertFalse(score_response(failed, case, catalog, 1)["correct_rejection"])
