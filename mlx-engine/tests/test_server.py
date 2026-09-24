"""Contract tests for the mlx-engine HTTP surface.

Four callers depend on this server's shape — the Go CLI via pkg/mlx, the MCP
speak tool, scripts/speak.sh in bash, and the Ava menu bar app's health poll —
and until now nothing checked any of it. It is TTS-only: the STT half moved out
(see TestHealth).
"""

import re
import sys
import threading
import time
import types
from pathlib import Path

import pytest
from conftest import FAKE_SNAPSHOT
from fastapi import HTTPException
from fastapi.testclient import TestClient


class TestHealth:
    def test_reports_the_tts_model_and_its_load_state(self, client, server):
        """Kokoro is warmed at startup, so a healthy server is a ready one.
        This used to also report a lazily-loaded 4B STT model; that half moved
        out — ava monitor runs Voxtral in its own process for streaming, and
        one-shot transcription uses whisper.cpp, which was 13x faster.
        """
        res = client.get("/health")
        assert res.status_code == 200
        assert res.json() == {
            "status": "ok",
            "tts_model": server.TTS_MODEL_PATH,
            "tts_loaded": True,
        }

    def test_does_not_count_as_activity(self, client, server):
        """The menu bar app polls /health every few seconds. If that reset the
        idle timer, the 15-minute shutdown would never fire while the panel is
        open — which is the whole point of the timer. server.py documents this;
        nothing enforced it.
        """
        server.last_request_time = 0.0
        client.get("/health")
        assert server.last_request_time == 0.0, "/health must not touch the idle timer"

    def test_speak_does_count_as_activity(self, client, server, tmp_path):
        server.last_request_time = 0.0
        _stub_successful_tts(server, tmp_path)
        client.post("/speak", json={"text": "hello"})
        assert server.last_request_time > 0.0, "/speak must reset the idle timer"


class TestSpeakValidation:
    def test_empty_text_is_rejected_without_generating(self, client, server, tmp_path):
        gen = _stub_successful_tts(server, tmp_path)
        res = client.post("/speak", json={"text": ""})
        assert res.status_code == 400
        assert "empty" in res.json()["detail"].lower()
        assert gen.calls == [], "must reject empty text before running synthesis"

    def test_whitespace_only_text_is_rejected(self, client):
        assert client.post("/speak", json={"text": "   \n\t "}).status_code == 400

    def test_text_is_required(self, client):
        assert client.post("/speak", json={}).status_code == 422

    def test_defaults_match_the_documented_contract(self, client, server, tmp_path):
        """af_heart / speed 1.0 are what every caller relies on when it sends
        only `text`. scripts/voice-defaults.json and VoiceSettings.swift both
        assume it."""
        gen = _stub_successful_tts(server, tmp_path)
        client.post("/speak", json={"text": "hi"})
        assert gen.last["voice"] == str(FAKE_SNAPSHOT / "voices" / "af_heart.safetensors")
        assert gen.last["speed"] == 1.0


class TestSpeakLanguageCode:
    """The voice id's first letter selects the phonemizer locale. Without it
    every voice is phonemized as American English, so a British voice speaks
    with the wrong accent's rules while still sounding like the right voice —
    a bug that is invisible unless you listen for it.
    """

    def test_derives_lang_code_from_the_voice_prefix(self, client, server, tmp_path):
        gen = _stub_successful_tts(server, tmp_path)
        for voice, expected in [
            ("af_heart", "a"),
            ("am_adam", "a"),
            ("bf_emma", "b"),
            ("bm_george", "b"),
            ("ef_dora", "e"),
        ]:
            client.post("/speak", json={"text": "hi", "voice": voice})
            assert gen.last["lang_code"] == expected, f"{voice} should phonemize as {expected!r}"

    def test_falls_back_to_american_english_for_an_empty_voice(self, client, server, tmp_path):
        gen = _stub_successful_tts(server, tmp_path)
        client.post("/speak", json={"text": "hi", "voice": ""})
        assert gen.last["lang_code"] == "a"


class TestSpeakLongText:
    def test_requests_joined_audio(self, client, server, tmp_path):
        """Kokoro splits long text into speech_000.wav, speech_001.wav, ... and
        the handler reads back only speech.wav. Without join_audio the response
        would silently truncate anything past a paragraph or two.
        """
        gen = _stub_successful_tts(server, tmp_path)
        client.post("/speak", json={"text": "a paragraph. " * 500})
        assert gen.last["join_audio"] is True


class TestSpeakFailureModes:
    def test_generation_failure_becomes_a_500_with_the_reason(self, client, server):
        server.tts_model.instance = "loaded"
        server.generate_audio = _raiser(RuntimeError("out of memory"))
        res = client.post("/speak", json={"text": "hi"})
        assert res.status_code == 500
        assert "out of memory" in res.json()["detail"]

    def test_silent_generation_is_an_error_not_an_empty_200(self, client, server):
        """generate_audio can return without writing a file. Returning 200 with
        a zero-byte body would make every caller think it spoke."""
        server.tts_model.instance = "loaded"
        server.generate_audio = lambda **kw: None  # writes nothing
        res = client.post("/speak", json={"text": "hi"})
        assert res.status_code == 500
        assert "no audio" in res.json()["detail"].lower()

    def test_returns_wav_bytes_on_success(self, client, server, tmp_path):
        _stub_successful_tts(server, tmp_path, audio=b"RIFFfake")
        res = client.post("/speak", json={"text": "hi"})
        assert res.status_code == 200
        assert res.headers["content-type"] == "audio/wav"
        assert res.content == b"RIFFfake"


class TestLazyModel:
    def test_loads_once_and_caches(self, server, recording_stub):
        loader = recording_stub(result="model-object")
        lm = server.LazyModel("STT", "some/path", loader)

        assert lm.loaded is False
        assert lm.get() == "model-object"
        assert lm.loaded is True
        assert lm.get() == "model-object"
        assert len(loader.calls) == 1, "a second get() must not reload the model"

    def test_a_load_failure_is_a_500_naming_the_model(self, server, recording_stub):
        lm = server.LazyModel("TTS", "bad/path", recording_stub(raises=OSError("no such model")))
        with pytest.raises(HTTPException) as caught:
            lm.get()
        assert caught.value.status_code == 500
        assert "TTS" in caught.value.detail
        assert "no such model" in caught.value.detail
        assert lm.loaded is False, "a failed load must not be cached as loaded"


class TestIdleShutdown:
    def test_pid_file_comes_from_the_generated_protocol(self, server):
        """The idle self-exit removes PID_FILE. It used to be a hardcoded
        "/tmp/voxtral-server.pid" while scripts/mlx-engine-server.sh wrote
        "/tmp/mlx-engine-server.pid", so shutting down on idle cleaned up a file
        nothing created and left the real one behind.

        Both sides now derive from internal/protocol/protocol.json, so the test
        is that neither has drifted back to a literal: server.py's default is
        the generated constant, and the start script assigns from the generated
        shell variable rather than spelling a path.
        """
        import protocol

        assert server.PID_FILE == protocol.ENGINE_PID_FILE

        script = (
            Path(__file__).resolve().parents[2] / "scripts" / "mlx-engine-server.sh"
        ).read_text()
        assert "source" in script and "protocol.sh" in script, (
            "scripts/mlx-engine-server.sh no longer sources the generated "
            "protocol.sh, so its pid path can drift from server.py's again"
        )
        match = re.search(r'^PID_FILE="\$\{AVA_ENGINE_PID_FILE:-([^}]*)\}"', script, re.MULTILINE)
        assert match, "scripts/mlx-engine-server.sh no longer assigns PID_FILE="
        assert match.group(1) == "$ENGINE_PID_FILE", (
            f"the start script sets PID_FILE={match.group(1)!r} instead of the "
            "generated $ENGINE_PID_FILE — a second spelling is how these two "
            "disagreed the first time"
        )

    def test_pid_file_follows_the_env_the_start_script_sets(self, monkeypatch):
        monkeypatch.setenv("MLX_ENGINE_PID_FILE", "/tmp/somewhere-else.pid")
        import sys

        sys.modules.pop("server", None)
        import server as reloaded

        assert reloaded.PID_FILE == "/tmp/somewhere-else.pid"

    def test_timeout_is_fifteen_minutes(self, server):
        assert server.IDLE_TIMEOUT_SEC == 15 * 60

    def test_last_request_time_starts_populated(self, server):
        """Starting at 0 would make the server look 56 years idle and shut down
        on the first check, before anyone connected."""
        assert server.last_request_time > time.time() - 60


def _stub_successful_tts(server, tmp_path, audio: bytes = b"RIFF" + b"\x00" * 44):
    """Point the TTS path at a stub that writes a file where the handler looks."""
    server.tts_model.instance = "loaded"

    class _Gen:
        def __init__(self):
            self.calls = []

        def __call__(self, **kwargs):
            self.calls.append(kwargs)
            out = kwargs["output_path"]
            with open(f"{out}/{kwargs['file_prefix']}.wav", "wb") as f:
                f.write(audio)

        @property
        def last(self):
            return self.calls[-1]

    gen = _Gen()
    server.generate_audio = gen
    return gen


def _raiser(exc):
    def _f(**kwargs):
        raise exc

    return _f


class TestStartupWarmUp:
    """The eager TTS warm-up is a performance contract, not an implementation
    detail: without it the first real /speak pays ~2.6s of MLX compilation, and
    so does the first one after every idle shutdown."""

    def test_startup_generates_once_to_warm_kokoro(self, server):
        calls = []
        server.generate_audio = lambda **kw: calls.append(kw)
        with TestClient(server.app):
            pass
        assert len(calls) == 1, "startup should warm the TTS model exactly once"
        assert calls[0]["voice"] == str(FAKE_SNAPSHOT / "voices" / "af_heart.safetensors")
        assert server.tts_model.loaded


class TestRequestSurface:
    """mlx_audio's generate_audio() advertises ~25 parameters. Almost none of
    them are Kokoro's: its own generate() takes text, voice, speed, lang_code
    and split_pattern. The rest belong to other TTS model types and are silently
    dropped, which made a request carrying `temperature` look accepted."""

    def test_unknown_fields_are_rejected_not_ignored(self, client):
        res = client.post("/speak", json={"text": "hi", "temperature": 1.5})
        assert res.status_code == 422
        assert "temperature" in str(res.json()["detail"])

    def test_the_three_real_knobs_are_accepted(self, client, server, tmp_path):
        gen = _stub_successful_tts(server, tmp_path)
        res = client.post("/speak", json={"text": "hi", "voice": "bf_emma", "speed": 1.4})
        assert res.status_code == 200
        assert gen.last["voice"] == str(FAKE_SNAPSHOT / "voices" / "bf_emma.safetensors")
        assert gen.last["speed"] == 1.4

    def test_split_pattern_is_forwarded_when_given(self, client, server, tmp_path):
        gen = _stub_successful_tts(server, tmp_path)
        client.post("/speak", json={"text": "a. b. c.", "split_pattern": r"\. "})
        assert gen.last["split_pattern"] == r"\. "

    def test_split_pattern_is_omitted_when_absent(self, client, server, tmp_path):
        """Kokoro has its own default. Passing None would override it with
        nothing rather than leaving it alone."""
        gen = _stub_successful_tts(server, tmp_path)
        client.post("/speak", json={"text": "hi"})
        assert "split_pattern" not in gen.last


class TestSpeakDoesNotBlockHealth:
    """generate_audio is blocking. Called from an `async def` handler it ran on
    the event loop, so /health went unanswered for as long as a synthesis
    took. The Go speaker gives /health 2s, and the menu bar polls it, so during
    a long sentence a second speaker concluded the engine was down and fell
    back to `say`, and the menu bar showed Stopped.
    """

    def test_health_answers_while_speak_is_synthesizing(self, client, server, tmp_path):
        def slow_generate(**kwargs):
            time.sleep(1.0)
            (Path(kwargs["output_path"]) / f"{kwargs['file_prefix']}.wav").write_bytes(b"RIFF")

        server.generate_audio = slow_generate
        speaking = threading.Thread(
            target=client.post,
            args=("/speak",),
            kwargs={"json": {"text": "a long sentence", "voice": "af_heart"}},
        )
        speaking.start()
        time.sleep(0.2)

        started = time.monotonic()
        resp = client.get("/health")
        waited = time.monotonic() - started
        speaking.join()

        assert resp.status_code == 200
        assert waited < 0.5, f"/health took {waited:.2f}s while /speak was synthesizing"

    def test_concurrent_speaks_still_synthesize_one_at_a_time(self, client, server, tmp_path):
        """Moving synthesis off the event loop must not let two requests into
        the model at once; MLX inference is not written to be re-entered."""
        active = []
        overlap = []

        def tracked_generate(**kwargs):
            active.append(1)
            if len(active) > 1:
                overlap.append(True)
            time.sleep(0.3)
            active.pop()
            (Path(kwargs["output_path"]) / f"{kwargs['file_prefix']}.wav").write_bytes(b"RIFF")

        server.generate_audio = tracked_generate
        threads = [
            threading.Thread(
                target=client.post,
                args=("/speak",),
                kwargs={"json": {"text": f"line {i}", "voice": "af_heart"}},
            )
            for i in range(3)
        ]
        for t in threads:
            t.start()
        for t in threads:
            t.join()
        assert not overlap, "two /speak requests ran generate_audio at the same time"


class TestModelSource:
    """Kokoro used to come from two repos, neither pinned: the weights from
    mlx-community on first load, and each voice from prince-canuma the first
    time someone picked it. `ava setup` could not make the engine complete, and
    a machine offline could not use a voice it had never spoken with. There is
    now one snapshot, at one revision, and everything is read from it."""

    def test_fetches_one_snapshot_at_a_pinned_revision(self, server):
        get_model_path = sys.modules["mlx_audio.utils"].get_model_path
        assert server.fetch_tts_model() == FAKE_SNAPSHOT
        assert get_model_path.last == {
            "path_or_hf_repo": server.TTS_MODEL_PATH,
            "revision": server.TTS_MODEL_REVISION,
        }
        assert re.fullmatch(r"[0-9a-f]{40}", server.TTS_MODEL_REVISION), (
            "pin a commit, not a branch: a branch moves under you"
        )

    def test_loads_the_model_at_that_same_revision(self, server):
        # By repo id: mlx_audio names the model type after the path it is
        # given, and a snapshot's directory is the revision hash. The first
        # version of this passed the snapshot path, and the real engine failed
        # to start with "Model type a71e4d38... not supported".
        load_model = sys.modules["mlx_audio.tts.utils"].load_model
        server.tts_model.get()
        assert load_model.last == {
            "model_path": server.TTS_MODEL_PATH,
            "revision": server.TTS_MODEL_REVISION,
        }

    def test_reads_voices_from_that_snapshot(self, client, server):
        calls = []
        server.generate_audio = lambda **kw: calls.append(kw)
        client.post("/speak", json={"text": "hi", "voice": "bf_emma,af_heart"})
        voices = FAKE_SNAPSHOT / "voices"
        assert (
            calls[-1]["voice"]
            == f"{voices / 'bf_emma.safetensors'},{voices / 'af_heart.safetensors'}"
        )
        assert calls[-1]["lang_code"] == "b", "the language still comes from the voice id"

    def test_fetch_command_downloads_the_snapshot_and_says_where(self, server, capsys):
        assert server.main(["fetch"]) == 0
        assert str(FAKE_SNAPSHOT) in capsys.readouterr().out


class TestFetchCheck:
    """`ava setup --check`, and through it the menu bar app's Set up button,
    ask the engine whether the model is there rather than knowing the Hugging
    Face cache layout and the pinned revision themselves. It must never
    download: it runs every time the app starts."""

    @pytest.fixture
    def hub(self, monkeypatch, recording_stub):
        hub = types.ModuleType("huggingface_hub")
        monkeypatch.setitem(sys.modules, "huggingface_hub", hub)
        return hub

    def test_a_cached_model_passes(self, server, hub, recording_stub):
        hub.snapshot_download = recording_stub(result=str(FAKE_SNAPSHOT))
        assert server.main(["fetch", "--check"]) == 0
        assert hub.snapshot_download.last == {
            "repo_id": server.TTS_MODEL_PATH,
            "revision": server.TTS_MODEL_REVISION,
            "local_files_only": True,
        }

    def test_a_missing_model_fails_without_downloading(self, server, hub, recording_stub, capsys):
        hub.snapshot_download = recording_stub(raises=FileNotFoundError("not cached"))
        get_model_path = sys.modules["mlx_audio.utils"].get_model_path
        before = len(get_model_path.calls)
        assert server.main(["fetch", "--check"]) == 1
        assert len(get_model_path.calls) == before, "the check downloaded"
        assert "not downloaded" in capsys.readouterr().out
