import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

import stop  # noqa: E402

STOP_PY = Path(__file__).resolve().parent.parent / "stop.py"


def _args(**overrides):
    ns = stop.build_parser().parse_args(
        ["--max-chars", str(overrides.pop("max_chars", 600))]
        + [f"--{k.replace('_', '-')}={v}" for k, v in overrides.items()]
    )
    return ns


def test_resolve_snippet_verbatim_under_cap():
    args = _args(max_chars=600)
    assert stop.resolve_snippet("A short reply.", args) == "A short reply."


def test_resolve_snippet_empty_input():
    args = _args(max_chars=600)
    assert stop.resolve_snippet("   ", args) == ""


def test_resolve_snippet_truncates_over_cap_without_llm_summary():
    long_text = "First sentence goes here with plenty of words to pad it out. " * 10
    long_text += "This is the true final sentence."
    args = _args(max_chars=100, llm_summary="false")
    result = stop.resolve_snippet(long_text, args)
    assert result.endswith("This is the true final sentence.")
    assert result != long_text.strip()


def test_resolve_snippet_unlimited_returns_full_text():
    long_text = "Word " * 300 + "Final sentence."
    args = _args(max_chars=0)
    result = stop.resolve_snippet(long_text, args)
    assert result.strip() == " ".join(long_text.split())


def test_cli_smoke_via_subprocess():
    result = subprocess.run(
        [sys.executable, str(STOP_PY), "--max-chars", "600", "--llm-summary", "false"],
        input="Hello from a test.",
        capture_output=True,
        text=True,
        check=True,
    )
    assert result.stdout.strip() == "Hello from a test."
