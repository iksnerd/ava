import asyncio
import contextlib
import functools
import os
import sys
import tempfile
import threading
import time
from pathlib import Path

from fastapi import FastAPI, HTTPException
from fastapi.responses import Response
from mlx_audio.tts.generate import generate_audio
from mlx_audio.tts.utils import load_model as load_tts_model
from mlx_audio.utils import get_model_path
from pydantic import BaseModel, ConfigDict

import protocol


@contextlib.asynccontextmanager
async def lifespan(_app):
    # Kokoro is warmed eagerly: measured
    # on an M3 Pro, Kokoro's first generate_audio call after loading costs an
    # extra ~2.6s (MLX's lazy compilation) beyond the ~230-290ms steady-state
    # a warm call takes — worth paying once at startup (delaying /health)
    # rather than on whichever real request happens to be first, including
    # the one right after the idle-shutdown timer below has torn things down.
    idle = asyncio.create_task(idle_shutdown_checker())
    _warm_tts_model()
    yield
    idle.cancel()


app = FastAPI(title="Local Kokoro TTS Server", lifespan=lifespan)

TTS_MODEL_PATH = "mlx-community/Kokoro-82M-bf16"
# Pinned: the snapshot holds the weights, the config and every voice, and all
# of it is read from here. Kokoro used to fetch each voice from a second repo
# (prince-canuma/Kokoro-82M) the first time it was used, so neither `ava
# setup` nor an offline machine could have the whole engine.
TTS_MODEL_REVISION = "a71e4d38b236d968966a2002c4c895dbd12b1c3c"
last_request_time = time.time()
IDLE_TIMEOUT_SEC = 15 * 60  # 15 minutes

# Which pid file to remove when shutting down on idle. scripts/mlx-engine-server.sh
# passes the path it actually writes; the default matches that script so a
# hand-started server still cleans up after itself. This was a hardcoded
# "/tmp/voxtral-server.pid" — a path nothing else used — so every idle self-exit
# left the real pid file on disk.
PID_FILE = os.environ.get("MLX_ENGINE_PID_FILE", protocol.ENGINE_PID_FILE)


class SpeakRequest(BaseModel):
    # extra="forbid" so a caller who sends a knob this server does not have gets
    # a 422 instead of a silent no-op. mlx_audio's generate_audio() accepts a
    # long list of parameters — temperature, cfg_scale, ddpm_steps, ref_audio
    # and more — but almost all of them belong to other TTS model types.
    # Kokoro's own generate() takes text, voice, speed, lang_code and
    # split_pattern, and nothing else. Passing `temperature` used to return 200
    # and change nothing.
    model_config = ConfigDict(extra="forbid")

    text: str
    voice: str = "af_heart"
    speed: float = 1.0
    # Kokoro's own chunking regex. Its default splits on blank lines, which is
    # wrong for text that arrives as one long paragraph — that lands as a single
    # chunk and hits the token cap. Exposed because it is one of the five things
    # Kokoro's generate() actually accepts; see docs/tuning.md.
    split_pattern: str | None = None


class LazyModel:
    """Loads a model into unified memory on first use and caches it, so the
    load/time/error-handle dance (and the print statements) lives in one
    place rather than inline in the startup warmup and /speak."""

    def __init__(self, label: str, model_path: str, loader):
        self.label = label
        self.model_path = model_path
        self._loader = loader
        self.instance = None

    def get(self):
        if self.instance is None:
            print(f"Loading {self.label} model: {self.model_path} into unified memory...")
            start_time = time.time()
            try:
                self.instance = self._loader(self.model_path)
                print(
                    f"{self.label} model loaded successfully in {time.time() - start_time:.2f} seconds."
                )
            except Exception as e:
                raise HTTPException(
                    status_code=500, detail=f"Failed to load {self.label} model: {e}"
                ) from e
        return self.instance

    @property
    def loaded(self) -> bool:
        return self.instance is not None


@functools.cache
def fetch_tts_model() -> Path:
    """The pinned Kokoro snapshot, downloaded into the Hugging Face cache if it
    is not there yet. The server loads from it and `fetch` (run by `ava setup`
    through mlx-engine-server.sh) downloads it, so the two cannot disagree."""
    return Path(get_model_path(path_or_hf_repo=TTS_MODEL_PATH, revision=TTS_MODEL_REVISION))


def voice_files(voice: str) -> str:
    """Maps a voice id, or a comma-separated blend, to the snapshot's voice
    files. Kokoro loads a value ending in .safetensors as a file instead of
    downloading the voice from its own default repo."""
    voices = fetch_tts_model() / "voices"
    return ",".join(str(voices / f"{v.strip()}.safetensors") for v in voice.split(","))


# The repo id, not the snapshot path: mlx_audio names the model type after the
# path it is given, and the snapshot's directory is the revision hash. Given the
# id and revision it resolves the same snapshot fetch_tts_model() does.
tts_model = LazyModel(
    "TTS",
    TTS_MODEL_PATH,
    lambda repo: load_tts_model(model_path=repo, revision=TTS_MODEL_REVISION),
)


async def idle_shutdown_checker():
    """Background task that shuts down the server after a period of inactivity."""
    while True:
        await asyncio.sleep(60)  # Check every minute
        idle_time = time.time() - last_request_time
        if idle_time > IDLE_TIMEOUT_SEC:
            print(
                f"Server idle for {IDLE_TIMEOUT_SEC / 60:.0f} minutes. Shutting down to free RAM..."
            )
            if os.path.exists(PID_FILE):
                with contextlib.suppress(OSError):
                    os.remove(PID_FILE)
            os._exit(0)


def _warm_tts_model():
    print("Warming up TTS model...")
    start = time.time()
    with tempfile.TemporaryDirectory() as tmpdir:
        generate_audio(
            text="Ready.",
            model=tts_model.get(),
            voice=voice_files("af_heart"),
            speed=1.0,
            output_path=tmpdir,
            file_prefix="warmup",
            audio_format="wav",
            join_audio=True,
            save=True,
            play=False,
            verbose=False,
        )
    print(f"TTS model warmed up in {time.time() - start:.2f}s.")


@app.get("/health")
async def health_check():
    # Deliberately does NOT touch last_request_time: the menu bar app polls
    # this every few seconds, and doing so would defeat the idle-shutdown
    # timer for as long as it's open. Only real work (/speak) counts as
    # activity.
    return {
        "status": "ok",
        "tts_model": TTS_MODEL_PATH,
        "tts_loaded": tts_model.loaded,
    }


# One synthesis at a time. /speak is a plain `def`, so FastAPI runs it on a
# worker thread and the event loop stays free to answer /health; it used to be
# `async def`, and the blocking generate_audio held the loop for the whole
# synthesis, so a concurrent speaker's health check timed out and it fell back
# to `say`. The lock keeps the old single-flight behaviour: MLX inference is
# not written to be re-entered, and the lazy model load must not run twice.
synthesis_lock = threading.Lock()


@app.post("/speak")
def speak(req: SpeakRequest):
    global last_request_time
    last_request_time = time.time()

    if not req.text.strip():
        raise HTTPException(status_code=400, detail="text must not be empty.")

    with synthesis_lock:
        return _synthesize(req)


def _synthesize(req: SpeakRequest) -> Response:
    tts_model_instance = tts_model.get()

    # Kokoro voice ids are prefixed by language+locale (af_/am_ = American
    # English, bf_/bm_ = British English, ef_/em_ = Spanish, etc.) — without
    # this, generate_audio's lang_code always defaults to American English,
    # so a British (or other-locale) voice gets phonemized with the wrong
    # accent's rules despite sounding like the right voice.
    lang_code = req.voice[:1] or "a"

    with tempfile.TemporaryDirectory() as tmpdir:
        prefix = "speech"
        try:
            generate_audio(
                text=req.text,
                model=tts_model_instance,
                voice=voice_files(req.voice),
                speed=req.speed,
                lang_code=lang_code,
                **({"split_pattern": req.split_pattern} if req.split_pattern else {}),
                output_path=tmpdir,
                file_prefix=prefix,
                audio_format="wav",
                # Kokoro splits text longer than ~1200 tokens into multiple
                # segments; without join_audio, generate_audio writes each as
                # its own speech_000.wav/speech_001.wav/... and reading back
                # only the first would silently truncate anything past a
                # paragraph or two. join_audio=True stitches them into one
                # speech.wav so arbitrarily long text (e.g. a whole article)
                # comes back complete.
                join_audio=True,
                save=True,
                play=False,
                verbose=False,
            )
        except Exception as e:
            raise HTTPException(status_code=500, detail=f"Speech generation failed: {e}") from e

        wav_path = os.path.join(tmpdir, f"{prefix}.wav")
        if not os.path.exists(wav_path):
            raise HTTPException(status_code=500, detail="Speech generation produced no audio.")
        with open(wav_path, "rb") as f:
            audio_bytes = f.read()

    return Response(content=audio_bytes, media_type="audio/wav")


def tts_model_cached() -> bool:
    """Whether the pinned snapshot is already in the Hugging Face cache.
    Never downloads: `ava setup --check` runs this every time the menu bar app
    starts."""
    from huggingface_hub import snapshot_download

    try:
        snapshot_download(
            repo_id=TTS_MODEL_PATH, revision=TTS_MODEL_REVISION, local_files_only=True
        )
    except Exception:
        return False
    return True


def main(argv: list[str]) -> int:
    """`python server.py fetch` downloads the model and prints where it is;
    `fetch --check` only says whether it is there. With no arguments, it
    serves."""
    if argv == ["fetch"]:
        print(fetch_tts_model())
        return 0
    if argv == ["fetch", "--check"]:
        if tts_model_cached():
            print("Kokoro model: downloaded")
            return 0
        print("Kokoro model: not downloaded")
        return 1
    if argv:
        print(f"usage: {sys.argv[0]} [fetch [--check]]", file=sys.stderr)
        return 2

    import uvicorn

    _host, _port = protocol.ENGINE_URL.rsplit("//", 1)[1].split(":")
    uvicorn.run(app, host=_host, port=int(_port))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
