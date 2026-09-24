"""Test harness for server.py that does not need MLX.

server.py imports mlx_audio's TTS entry points at module level, and those only
exist inside the Apple-Silicon venv that `make setup-deps` builds —
146 packages including torch and the CUDA stack. Importing the real thing to
test HTTP routing would make the suite cost gigabytes and refuse to run on
anything but an M-series Mac.

So the MLX surface gets stubbed in sys.modules before server is imported. What
is left is the part worth testing: request validation, the response contract,
the idle-shutdown bookkeeping, and LazyModel's caching and error wrapping. The
model inference itself is not tested here and cannot be — that needs the real
weights, and `POST /speak` against a running server is the check for that.
"""

import sys
import types
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))


class _RecordingStub:
    """Records the kwargs it was called with, so a test can assert on them."""

    def __init__(self, result=None, raises=None):
        self.calls = []
        self.result = result
        self.raises = raises

    def __call__(self, *args, **kwargs):
        self.calls.append(kwargs or (args[0] if args else None))
        if self.raises is not None:
            raise self.raises
        return self.result

    @property
    def last(self):
        return self.calls[-1]


FAKE_SNAPSHOT = Path("/fake/hf-cache/kokoro-snapshot")


def _install_mlx_stubs():
    for name in (
        "mlx_audio",
        "mlx_audio.utils",
        "mlx_audio.tts",
        "mlx_audio.tts.generate",
        "mlx_audio.tts.utils",
    ):
        sys.modules.setdefault(name, types.ModuleType(name))

    # Where the pinned Kokoro snapshot would land in the Hugging Face cache.
    sys.modules["mlx_audio.utils"].get_model_path = _RecordingStub(result=FAKE_SNAPSHOT)

    sys.modules["mlx_audio.tts.generate"].generate_audio = _RecordingStub()
    sys.modules["mlx_audio.tts.utils"].load_model = _RecordingStub(result="tts-model")


_install_mlx_stubs()


@pytest.fixture
def server():
    """A freshly reloaded server module, so module-level state (the lazy model
    caches and last_request_time) never leaks between tests."""
    for mod in list(sys.modules):
        if mod == "server":
            del sys.modules[mod]
    import server as mod

    return mod


@pytest.fixture
def client(server):
    from fastapi.testclient import TestClient

    # Context-managed so FastAPI startup/shutdown events fire the way they do in
    # production — the idle-shutdown task is started there.
    with TestClient(server.app) as c:
        yield c


@pytest.fixture
def recording_stub():
    return _RecordingStub
