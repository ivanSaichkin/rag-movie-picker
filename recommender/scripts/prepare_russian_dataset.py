"""Overlay Russian TMDB metadata by ID, retaining the original catalogue and ratings."""

import argparse
import csv
import hashlib
import json
from pathlib import Path


def prepare(source: Path, enrichment: Path, destination: Path) -> dict:
    if destination.resolve() in {source.resolve(), enrichment.resolve()}:
        raise ValueError("Output must not overwrite an input")
    report = json.loads(enrichment.read_text(encoding="utf-8"))
    translated = {int(movie["movie_id"]): movie for movie in report["movies"]}
    if len(translated) != len(report["movies"]):
        raise ValueError("Duplicate IDs in enrichment report")
    with source.open(encoding="utf-8-sig", newline="") as stream:
        reader = csv.DictReader(stream)
        fields = reader.fieldnames
        rows = list(reader)
    counts = {"rows": len(rows), "russian_titles": 0, "russian_overviews": 0, "fallback_ids": []}
    for row in rows:
        translation = translated.get(int(row["id"]), {})
        for field in ("title", "overview"):
            value = (translation.get(field + "_ru") or "").strip()
            if value:
                row[field] = value
                counts["russian_" + field + "s"] += 1
            elif field == "overview":
                counts["fallback_ids"].append(int(row["id"]))
    counts["indexable_rows"] = sum(bool(row["overview"].strip()) for row in rows)
    counts["source_sha256"] = hashlib.sha256(source.read_bytes()).hexdigest()
    counts["enrichment_sha256"] = hashlib.sha256(enrichment.read_bytes()).hexdigest()
    destination.parent.mkdir(parents=True, exist_ok=True)
    temporary = destination.with_suffix(".tmp")
    with temporary.open("w", encoding="utf-8", newline="") as stream:
        writer = csv.DictWriter(stream, fieldnames=fields)
        writer.writeheader()
        writer.writerows(rows)
    temporary.replace(destination)
    counts["dataset_sha256"] = hashlib.sha256(destination.read_bytes()).hexdigest()
    destination.with_suffix(".coverage.json").write_text(
        json.dumps(counts, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    return counts


def main() -> None:
    directory = Path(__file__).resolve().parents[3] / "data" / "raw" / "TMDB 5000 Movie Dataset"
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", type=Path, default=directory / "tmdb_5000_movies.csv")
    parser.add_argument("--enrichment", type=Path, default=directory / "tmdb_enriched/reports/catalog-4803.json")
    parser.add_argument("--output", type=Path, default=directory / "tmdb_5000_movies_ru.csv")
    args = parser.parse_args()
    counts = prepare(args.source, args.enrichment, args.output)
    print(json.dumps({key: value for key, value in counts.items() if key != "fallback_ids"}, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
