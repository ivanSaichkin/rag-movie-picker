import json

import httpx

from movie_rag.models import Budget, ServiceError


class OpenRouterClient:
    """OpenAI-compatible JSON completion adapter; provider details stay here."""

    def __init__(
        self, client: httpx.Client, base_url: str, api_key: str, model: str, timeout: float = 20, max_tokens: int = 4096
    ) -> None:
        self.client, self.url, self.key, self.model = client, base_url.rstrip("/") + "/chat/completions", api_key, model
        self.timeout, self.max_tokens = timeout, max_tokens

    def complete_json(self, system: str, user: str, budget: Budget) -> dict:
        if not self.key or not self.model:
            raise ServiceError("UNAVAILABLE", "LLM is not configured")
        for attempt in range(2):
            timeout = min(self.timeout, budget.remaining())
            try:
                response = self.client.post(
                    self.url,
                    headers={"Authorization": f"Bearer {self.key}"},
                    json={
                        "model": self.model,
                        "temperature": 0,
                        "max_tokens": self.max_tokens,
                        "response_format": {"type": "json_object"},
                        "messages": [{"role": "system", "content": system}, {"role": "user", "content": user}],
                    },
                    timeout=timeout,
                )
                if response.status_code in {429, 502, 503, 504} and attempt == 0:
                    continue
                response.raise_for_status()
                result = json.loads(response.json()["choices"][0]["message"]["content"])
                if not isinstance(result, dict):
                    raise ValueError("JSON object expected")
                budget.remaining()
                return result
            except ServiceError:
                raise
            except (httpx.HTTPError, ValueError, KeyError, TypeError, IndexError) as exc:
                budget.remaining()
                if attempt == 0 and isinstance(exc, (httpx.ConnectError, httpx.TimeoutException)):
                    continue
                raise ServiceError("UNAVAILABLE", "LLM completion failed") from exc
        raise ServiceError("UNAVAILABLE", "LLM completion failed")
