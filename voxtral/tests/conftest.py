"""Test harness for realtime.py.

realtime.py imports sounddevice at module level, which needs PortAudio and a
real audio system — neither exists on a CI runner. It is stubbed here so the
parts that are pure computation can be tested: the high-pass filter and
--device resolution.

scipy and numpy are real, not stubbed. The filter is the thing under test and a
fake would prove nothing about it.
"""

import sys
import types
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))


class FakeSoundDevice(types.ModuleType):
    """Minimal stand-in for the sounddevice API realtime.py actually touches."""

    def __init__(self, devices=None, default_input=0):
        super().__init__("sounddevice")
        self._devices = devices if devices is not None else []
        self.default = types.SimpleNamespace(device=(default_input, 1))

    def query_devices(self, idx=None):
        if idx is None:
            return self._devices
        return self._devices[idx]

    # realtime.py only constructs these in the streaming paths, which are not
    # under test; present so an accidental import-time reference does not blow up.
    def InputStream(self, **kwargs):  # noqa: N802 - mirrors the real API's name
        raise AssertionError("tests must not open an audio stream")


def _device(name, in_ch=1, rate=16000):
    return {"name": name, "max_input_channels": in_ch, "default_samplerate": rate}


@pytest.fixture
def make_devices():
    return _device


@pytest.fixture
def realtime(monkeypatch):
    """Import realtime.py against a fake sounddevice, and let a test swap the
    device list per case."""

    def _load(devices=None, default_input=0):
        fake = FakeSoundDevice(devices=devices, default_input=default_input)
        monkeypatch.setitem(sys.modules, "sounddevice", fake)
        sys.modules.pop("realtime", None)
        import realtime as mod

        return mod

    return _load
