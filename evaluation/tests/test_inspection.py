import unittest
from types import SimpleNamespace
from unittest.mock import Mock

from benchmark import parse_arguments
from movie_eval.client import RecommenderClient, pb
from movie_eval.goldenset import MovieFilters


class InspectionTests(unittest.TestCase):
    def test_inspection_preserves_usage_and_errors(self):
        case = SimpleNamespace(query_id="test", query="space", filters=MovieFilters())
        for failed in (False, True):
            with self.subTest(failed=failed):
                client = RecommenderClient.__new__(RecommenderClient)
                client.timeout, client.disable_llm, client.inspect = 5, False, True
                trace = pb.RequestDiagnostics(
                    llm_calls=[
                        pb.LLMCallTrace(
                            stage="selection",
                            attempts=[pb.LLMAttempt(attempt=1, http_status=200, provider_response='{"usage":{"cost":0.01}}')],
                        )
                    ]
                )
                inspection = pb.RecommendationInspection(diagnostics=trace)
                if failed:
                    inspection.error_code, inspection.error_message = "UNAVAILABLE", "test failure"
                else:
                    inspection.response.CopyFrom(
                        pb.RecommendResponse(mode="empty", strategy_used="hybrid", explanation="No match", has_matches=False)
                    )
                client.stub = Mock()
                client.stub.InspectRecommendation.return_value = inspection
                result = client.recommend(case, "hybrid", 10, True)
                self.assertEqual(result["status"], "UNAVAILABLE" if failed else "OK")
                self.assertIn('"cost":0.01', result["diagnostics"]["llm_calls"][0]["attempts"][0]["provider_response"])
                client.stub.Recommend.assert_not_called()
                if not failed:
                    self.assertIs(result["has_matches"], False)

    def test_inspection_cannot_be_used_for_load(self):
        with self.assertRaises(SystemExit):
            parse_arguments(["--inspect", "--mode", "load"])


if __name__ == "__main__":
    unittest.main()
