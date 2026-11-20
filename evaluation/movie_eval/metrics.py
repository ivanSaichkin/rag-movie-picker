import math
from collections.abc import Mapping, Sequence


def ranking_metrics(
    ranked_ids: Sequence[int],
    judgments: Mapping[int, int],
    k: int,
    threshold: int = 2,
    invalid_ids: frozenset[int] = frozenset(),
) -> dict[str, float]:
    """Unjudged/invalid documents score zero; duplicates consume a rank but gain nothing."""
    if k < 1 or threshold < 1:
        raise ValueError("k and relevance threshold must be positive")
    seen = set()
    grades = []
    for movie_id in ranked_ids[:k]:
        grade = judgments.get(movie_id, 0) if movie_id not in seen and movie_id not in invalid_ids else 0
        grades.append(grade)
        seen.add(movie_id)
    relevant_count = sum(grade >= threshold for grade in judgments.values())
    hits = 0
    precision_sum = 0.0
    reciprocal_rank = 0.0
    dcg = 0.0
    for rank, grade in enumerate(grades, start=1):
        dcg += (2**grade - 1) / math.log2(rank + 1)
        if grade >= threshold:
            hits += 1
            precision_sum += hits / rank
            if not reciprocal_rank:
                reciprocal_rank = 1 / rank
    ideal = sorted(judgments.values(), reverse=True)[:k]
    idcg = sum((2**grade - 1) / math.log2(rank + 1) for rank, grade in enumerate(ideal, start=1))
    return {
        "ndcg_at_k": dcg / idcg if idcg else 0.0,
        "ap_at_k": precision_sum / min(relevant_count, k) if relevant_count else 0.0,
        "precision_at_k": hits / k,
        "recall_at_k": hits / relevant_count if relevant_count else 0.0,
        "reciprocal_rank_at_k": reciprocal_rank,
        "hit_at_k": float(hits > 0),
    }


def percentile(values: Sequence[float], percent: float) -> float | None:
    if not values:
        return None
    ordered = sorted(values)
    position = (len(ordered) - 1) * percent / 100
    lower, upper = math.floor(position), math.ceil(position)
    return ordered[lower] + (ordered[upper] - ordered[lower]) * (position - lower)
