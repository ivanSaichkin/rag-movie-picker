import httpx
import pytest

from movie_rag.llm import OpenRouterClient
from movie_rag.models import Budget, ServiceError


def test_llm_retry_and_json():
    calls = []

    def respond(request):
        calls.append(request)
        if len(calls) == 1:
            return httpx.Response(503)
        return httpx.Response(200, json={"choices": [{"message": {"content": '{"items":[]}'}}]})

    with httpx.Client(transport=httpx.MockTransport(respond)) as client:
        llm = OpenRouterClient(client, "https://example.test/v1", "secret", "test")
        assert llm.complete_json("system", "user", Budget.seconds(10)) == {"items": []}
    assert len(calls) == 2
    assert calls[0].url.path == "/v1/chat/completions"


@pytest.mark.parametrize("response", [httpx.Response(401), httpx.Response(200, json={"bad": True})])
def test_llm_errors_do_not_expose_provider_body(response):
    with httpx.Client(transport=httpx.MockTransport(lambda request: response)) as client:
        with pytest.raises(ServiceError, match="LLM completion failed"):
            OpenRouterClient(client, "https://example.test", "secret", "test").complete_json("s", "u", Budget.seconds(10))
