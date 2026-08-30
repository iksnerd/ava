import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from notify import resolve_message  # noqa: E402

NOTIFY_PY = Path(__file__).resolve().parent.parent / "notify.py"


def test_resolve_message_under_cap_unchanged():
    assert resolve_message("Claude needs your permission.", 500) == "Claude needs your permission."


def test_resolve_message_unlimited():
    msg = "x" * 1000
    assert resolve_message(msg, 0) == msg


def test_resolve_message_truncates_keeping_last_sentence():
    msg = "First part of a long notification message that goes on. Final sentence here."
    result = resolve_message(msg, 30)
    assert result == "Final sentence here."


def test_cli_smoke_via_subprocess():
    result = subprocess.run(
        [sys.executable, str(NOTIFY_PY), "--max-chars", "500"],
        input="Claude needs your permission to use Bash",
        capture_output=True,
        text=True,
        check=True,
    )
    assert result.stdout.strip() == "Claude needs your permission to use Bash"
