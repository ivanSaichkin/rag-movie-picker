"""Run the local SalarySense chat with the upstream RAG engine."""

import argparse
import json
import logging
import os
import sys
from collections.abc import Callable
from functools import partial
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

DEMO_DIR = Path(__file__).resolve().parent
FRONTEND = DEMO_DIR.parent / "frontend"
CORPUS = DEMO_DIR / "corpus.json"
HOST = "127.0.0.1"
MAX_QUESTION = 1000
MAX_BODY = 8192


def load_env() -> None:
    path = DEMO_DIR / ".env"
    if path.is_file():
        for line in path.read_text(encoding="utf-8-sig").splitlines():
            key, _, value = line.partition("=")
            key = key.strip()
            if key in {"OPENROUTER_API_KEY", "OPENROUTER_MODEL"}:
                os.environ.setdefault(key, value.strip().strip("\"'"))


def make_handler(ask: Callable[[str], dict], status: dict, port: int) -> type[SimpleHTTPRequestHandler]:
    hosts = {f"{HOST}:{port}", f"localhost:{port}"}

    class Handler(SimpleHTTPRequestHandler):
        def reply(self, code: int, data: dict) -> None:
            payload = json.dumps(data, ensure_ascii=False).encode("utf-8")
            self.send_response(code)
            self.send_header("Content-Type", "application/json; charset=utf-8")
            self.send_header("Content-Length", str(len(payload)))
            self.send_header("Cache-Control", "no-store")
            self.end_headers()
            self.wfile.write(payload)

        def send_head(self):
            if self.headers.get("Host") not in hosts:
                self.send_error(403)
                return None
            if self.path.split("?", 1)[0] in {"/", "/index.html"}:
                self.path = "/salarysense.html"
            return super().send_head()

        def do_GET(self) -> None:
            if self.path == "/api/status":
                if self.headers.get("Host") in hosts:
                    self.reply(200, status)
                else:
                    self.send_error(403)
                return
            super().do_GET()

        def do_POST(self) -> None:
            host = self.headers.get("Host")
            origin = self.headers.get("Origin")
            if host not in hosts or (origin and origin != f"http://{host}"):
                self.reply(403, {"error": "Разрешены запросы с локальной страницы."})
                return
            if self.path != "/api/ask":
                self.reply(404, {"error": "Маршрут не найден."})
                return
            if self.headers.get_content_type() != "application/json":
                self.reply(415, {"error": "Ожидается application/json."})
                return
            try:
                length = int(self.headers.get("Content-Length", "0"))
                if not 0 < length <= MAX_BODY:
                    raise ValueError
                body = json.loads(self.rfile.read(length).decode("utf-8"))
                question = body.get("question") if isinstance(body, dict) else None
                if not isinstance(question, str) or not 0 < len(question.strip()) <= MAX_QUESTION:
                    raise ValueError
                question = question.strip()
                question.encode("utf-8")
            except ValueError:
                self.reply(400, {"error": f"Введите вопрос длиной от 1 до {MAX_QUESTION} символов."})
                return
            self.reply(200, ask(question))

    return Handler


def main() -> None:
    parser = argparse.ArgumentParser(description="Локальный чат с движком SalarySense")
    parser.add_argument("--port", type=int, default=8090)
    args = parser.parse_args()
    if not 1 <= args.port <= 65535:
        parser.error("--port должен быть в диапазоне 1–65535")
    engine = DEMO_DIR / "engine"
    if not (engine / "app" / "backend" / "rag" / "answer.py").is_file():
        parser.exit(1, "Выполните git submodule update --init salarysense_demo/engine\n")

    sys.path.insert(0, str(engine))
    from app.backend.config import Settings
    from app.backend.rag.answer import REFUSAL, RagService
    from app.backend.rag.llm import LlmUnavailable

    load_env()
    settings = Settings(
        llm_api_key=os.getenv("OPENROUTER_API_KEY", "").strip(),
        llm_model=os.getenv("OPENROUTER_MODEL", "").strip() or Settings.llm_model,
        rag_top_k=4,
    )
    rag = RagService(settings, corpus_path=CORPUS)
    documents = rag.index.count

    def ask(question: str) -> dict:
        if settings.llm_configured:
            try:
                trace = rag.answer_with_trace(question)
                mode = "insufficient" if trace.refused else "answered"
                return {"mode": mode, "answer": trace.text, "sources": trace.sources}
            except LlmUnavailable:
                logging.getLogger(__name__).warning("OpenRouter unavailable. Showing BM25 results.")
                note = "OpenRouter сейчас недоступен. Ниже фрагменты, найденные BM25."
        else:
            note = "Режим BM25. Ниже фрагменты, найденные в базе знаний."

        hits = rag.retrieve(question)
        if not hits:
            return {"mode": "insufficient", "answer": REFUSAL, "sources": []}
        sources = [{"id": hit.id, "title": hit.document.title, "fragment": hit.document.text} for hit in hits]
        return {"mode": "retrieval_only", "answer": note, "sources": sources}

    status = {"documents": documents, "llm_enabled": settings.llm_configured}
    handler = make_handler(ask, status, args.port)
    with ThreadingHTTPServer((HOST, args.port), partial(handler, directory=str(FRONTEND))) as server:
        print(f"SalarySense: http://{HOST}:{args.port} — {documents} документов", flush=True)
        try:
            server.serve_forever()
        except KeyboardInterrupt:
            pass


if __name__ == "__main__":
    main()
