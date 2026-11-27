import json
import time

import httpx

from movie_rag.models import Budget, LLMAttempt, LLMCallTrace, ServiceError


class OpenRouterClient:
    """OpenAI-compatible JSON completion adapter; provider details stay here."""

    def __init__(
        self,
        client: httpx.Client,
        base_url: str,
        api_key: str,
        model: str,
        timeout: float = 20,
        max_tokens: int = 4096,
        *,
        reasoning_enabled: bool | None = None,
        structured_outputs: bool = False,
    ) -> None:
        self.client, self.url, self.key, self.model = client, base_url.rstrip("/") + "/chat/completions", api_key, model
        self.timeout, self.max_tokens = timeout, max_tokens
        self.reasoning_enabled, self.structured_outputs = reasoning_enabled, structured_outputs

    def complete_json(
        self, system: str, user: str, budget: Budget, *, schema: dict | None = None, trace: LLMCallTrace | None = None
    ) -> dict:
        payload = {
            "model": self.model,
            "temperature": 0,
            "max_tokens": self.max_tokens,
            "response_format": {"type": "json_object"},
            "messages": [{"role": "system", "content": system}, {"role": "user", "content": user}],
        }
        if self.structured_outputs:
            if schema is None:
                raise ServiceError("UNAVAILABLE", "Structured output requires a response schema")
            payload["response_format"] = {
                "type": "json_schema",
                "json_schema": {"name": "response", "strict": True, "schema": schema},
            }
        if self.reasoning_enabled is not None:
            payload["reasoning"] = {"enabled": self.reasoning_enabled}
        if self.structured_outputs or self.reasoning_enabled is not None:
            # Do not silently route to a provider that ignores the requested constraints.
            payload["provider"] = {"require_parameters": True}
        if trace is not None:
            trace.request_json, trace.request_truncated = self._safe_text(json.dumps(payload, ensure_ascii=False))
        if not self.key or not self.model:
            raise ServiceError("UNAVAILABLE", "LLM is not configured")
        for attempt in range(2):
            timeout = min(self.timeout, budget.remaining())
            detail = LLMAttempt(attempt=attempt + 1) if trace is not None else None
            if detail is not None:
                trace.attempts.append(detail)
            started = time.perf_counter()
            try:
                response = self.client.post(
                    self.url,
                    headers={"Authorization": f"Bearer {self.key}"},
                    json=payload,
                    timeout=timeout,
                )
                if detail is not None:
                    detail.http_status = response.status_code
                    detail.provider_response, detail.truncated = self._safe_text(response.text)
                if response.status_code in {429, 502, 503, 504} and attempt == 0:
                    if detail is not None:
                        detail.error = "Transient HTTP error; retrying"
                    continue
                response.raise_for_status()
                content = response.json()["choices"][0]["message"]["content"]
                if detail is not None:
                    detail.content, clipped = self._safe_text(content if isinstance(content, str) else json.dumps(content))
                    detail.truncated = detail.truncated or clipped
                result = json.loads(content)
                if not isinstance(result, dict):
                    raise ValueError("JSON object expected")
                budget.remaining()
                return result
            except ServiceError:
                if detail is not None:
                    detail.error = "Request budget expired or cancelled"
                raise
            except (httpx.HTTPError, ValueError, KeyError, TypeError, IndexError) as exc:
                if detail is not None:
                    detail.error = type(exc).__name__
                budget.remaining()
                if attempt == 0 and isinstance(exc, (httpx.ConnectError, httpx.TimeoutException)):
                    continue
                raise ServiceError("UNAVAILABLE", "LLM completion failed") from exc
            finally:
                if detail is not None:
                    detail.elapsed_ms = (time.perf_counter() - started) * 1000
        raise ServiceError("UNAVAILABLE", "LLM completion failed")

    def _safe_text(self, value: str) -> tuple[str, bool]:
        # Only diagnostics use this text. Never return authentication headers or the actual key.
        redacted = value.replace(self.key, "[REDACTED]") if self.key else value
        limit = 256 * 1024
        return redacted[:limit], len(redacted) > limit
