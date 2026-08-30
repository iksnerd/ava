import sys
from pathlib import Path

import httpx

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

import ollama_client  # noqa: E402
from ollama_client import ollama_summarize  # noqa: E402


def test_ollama_summarize_success(monkeypatch):
    def fake_post(url, json, timeout):
        request = httpx.Request("POST", url)
        return httpx.Response(200, json={"response": "  A short summary.  "}, request=request)

    monkeypatch.setattr(ollama_client.httpx, "post", fake_post)
    assert ollama_summarize("some long text", 600, "qwen2.5:3b") == "A short summary."


def test_ollama_summarize_timeout(monkeypatch):
    def fake_post(url, json, timeout):
        raise httpx.ConnectTimeout("timed out", request=httpx.Request("POST", url))

    monkeypatch.setattr(ollama_client.httpx, "post", fake_post)
    assert ollama_summarize("text", 600, "qwen2.5:3b") == ""


def test_ollama_summarize_connection_refused(monkeypatch):
    def fake_post(url, json, timeout):
        raise httpx.ConnectError("refused", request=httpx.Request("POST", url))

    monkeypatch.setattr(ollama_client.httpx, "post", fake_post)
    assert ollama_summarize("text", 600, "qwen2.5:3b") == ""


def test_ollama_summarize_non_2xx(monkeypatch):
    def fake_post(url, json, timeout):
        request = httpx.Request("POST", url)
        return httpx.Response(500, request=request)

    monkeypatch.setattr(ollama_client.httpx, "post", fake_post)
    assert ollama_summarize("text", 600, "qwen2.5:3b") == ""


def test_ollama_summarize_malformed_json(monkeypatch):
    def fake_post(url, json, timeout):
        request = httpx.Request("POST", url)
        return httpx.Response(200, content=b"not json", request=request)

    monkeypatch.setattr(ollama_client.httpx, "post", fake_post)
    assert ollama_summarize("text", 600, "qwen2.5:3b") == ""
