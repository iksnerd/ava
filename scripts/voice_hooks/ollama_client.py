"""httpx-based client for the local Ollama daemon.

Never raises — mirrors the old curl+python3-c behavior of returning "" on any
failure (unreachable, timeout, non-2xx, malformed JSON), since callers treat
an empty summary as "fall back to truncation," not an error to propagate.
"""

import httpx

DEFAULT_OLLAMA_URL = "http://127.0.0.1:11434"
DEFAULT_TIMEOUT = 12.0

_SYSTEM_PROMPT = (
    "You summarize Claude Code responses into one short sentence that will be "
    "read aloud by a text-to-speech engine, not shown as text. Rules:\n"
    '- Output ONLY the summary sentence itself: no preamble, no "Summary:", no surrounding quotes.\n'
    "- Write it as natural spoken language: avoid or spell out anything a TTS engine would "
    'mispronounce or read literally, such as abbreviations (say "for example" not "e.g."), '
    'symbols ("and" not "&"), and parentheticals.\n'
    "- One sentence, no line breaks."
)


def ollama_summarize(
    text: str,
    max_chars: int,
    model: str,
    *,
    base_url: str = DEFAULT_OLLAMA_URL,
    timeout: float = DEFAULT_TIMEOUT,
) -> str:
    words = max(8, max_chars // 6)
    prompt = f"Summarize the following in under {words} words:\n\n{text}"
    payload = {"model": model, "system": _SYSTEM_PROMPT, "prompt": prompt, "stream": False}
    try:
        resp = httpx.post(f"{base_url}/api/generate", json=payload, timeout=timeout)
        resp.raise_for_status()
        return resp.json().get("response", "").strip()
    except (httpx.HTTPError, ValueError):
        return ""
