"""Compute full-answer Accuracy from a manually reviewed benchmark run."""

import argparse
import json
from collections import defaultdict
from pathlib import Path


def grade(directory: Path) -> dict:
    def read(name: str) -> list[dict]:
        return [json.loads(line) for line in (directory / name).read_text(encoding="utf-8").splitlines() if line.strip()]

    def key(record: dict) -> tuple:
        return record["query_id"], record["strategy"], record["sequence"]

    reviews = read("answer_review.jsonl")
    by_key = {key(review): review for review in reviews}
    if len(by_key) != len(reviews):
        raise ValueError("Duplicate review rows")
    requests = [
        record for record in read("requests.jsonl") if record["phase"] == "quality" and record.get("kind", "normal") == "normal"
    ]
    expected = {(record["query_id"], record["requested_strategy"], record["sequence"]) for record in requests}
    if set(by_key) != expected:
        raise ValueError("Review rows do not match measured ordinary requests")
    groups = defaultdict(list)
    for record in requests:
        review = by_key[record["query_id"], record["requested_strategy"], record["sequence"]]
        value = review["answer_correct"]
        if value is not None and type(value) is not bool:
            raise ValueError("answer_correct must be true, false or null")
        groups[record["requested_strategy"]].append(False if record["status"] != "OK" else value)
    result = {}
    for strategy, values in groups.items():
        pending = sum(value is None for value in values)
        result[strategy] = {
            "requests": len(values),
            "correct": sum(value is True for value in values),
            "pending": pending,
            "accuracy": None if pending else sum(values) / len(values),
        }
    (directory / "answer_accuracy.json").write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
    return result


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("run_dir", type=Path)
    print(json.dumps(grade(parser.parse_args().run_dir), indent=2))
