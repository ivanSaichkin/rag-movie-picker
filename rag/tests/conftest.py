import pytest

from movie_rag.models import Catalog, Genre, Movie, ServiceError, VectorRepresentation


@pytest.fixture
def movies():
    return [
        Movie(
            movie_id=1,
            title="Space voyage",
            overview="space stars ship adventure",
            genres=[Genre(id=878, name="Science Fiction")],
            year=2014,
            rating=8.0,
            vote_count=100,
        ),
        Movie(
            movie_id=2,
            title="Garden",
            overview="garden family comedy",
            genres=[Genre(id=35, name="Comedy")],
            year=2001,
            rating=6.0,
            vote_count=20,
        ),
        Movie(movie_id=3, title="Unknown", overview="space alien mystery", genres=[Genre(id=878, name="Science Fiction")]),
    ]


@pytest.fixture
def catalog(movies):
    return Catalog(genres=[Genre(id=35, name="Comedy"), Genre(id=878, name="Science Fiction")], movie_count=len(movies))


class Ready:
    def check(self):
        return None


class FakeDense:
    dimension = 3
    max_length = 512
    truncated_documents = 0

    def encode_documents(self, texts):
        return [self.encode_query(text) for text in texts]

    def encode_query(self, text):
        return VectorRepresentation(dense=[1.0, 0.0, 0.0] if "space" in text else [0.0, 1.0, 0.0])


class BrokenEncoder:
    def encode_query(self, text):
        raise ServiceError("UNAVAILABLE", "encoder failed")


class ScriptedLLM:
    def __init__(self, replies):
        self.replies = list(replies)
        self.calls = []

    def complete_json(self, system, user, budget):
        budget.remaining()
        self.calls.append((system, user))
        reply = self.replies.pop(0)
        if isinstance(reply, Exception):
            raise reply
        return reply
