import asyncio
import contextlib
import os
import tempfile
import time

from fastapi import FastAPI, HTTPException
from fastapi.responses import Response
from mlx_audio.tts.generate import generate_audio
from mlx_audio.tts.utils import load_model as load_tts_model
from pydantic import BaseModel, ConfigDict

app = FastAPI(title="Local Kokoro TTS Server")

TTS_MODEL_PATH = "mlx-community/Kokoro-82M-bf16"
last_request_time = time.time()
IDLE_TIMEOUT_SEC = 15 * 60  # 15 minutes

# Which pid file to remove when shutting down on idle. scripts/mlx-engine-server.sh
# passes the path it actually writes; the default matches that script so a
# hand-started server still cleans up after itself. This was a hardcoded
# "/tmp/voxtral-server.pid" — a path nothing else used — so every idle self-exit
# left the real pid file on disk.
PID_FILE = os.environ.get("MLX_ENGINE_PID_FILE", "/tmp/mlx-engine-server.pid")


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


class LazyModel:
    """Loads a model into unified memory on first use and caches it —
    shared by the STT and TTS endpoints so the load/time/error-handle
    dance (and the print statements) only lives in one place."""

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


tts_model = LazyModel("TTS", TTS_MODEL_PATH, load_tts_model)


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


@app.on_event("startup")
async def startup_event():
    # Kokoro is warmed eagerly: measured
    # on an M3 Pro, Kokoro's first generate_audio call after loading costs an
    # extra ~2.6s (MLX's lazy compilation) beyond the ~230-290ms steady-state
    # a warm call takes — worth paying once at startup (delaying /health)
    # rather than on whichever real request happens to be first, including
    # the one right after the idle-shutdown timer below has torn things down.
    asyncio.create_task(idle_shutdown_checker())
    _warm_tts_model()


def _warm_tts_model():
    print("Warming up TTS model...")
    start = time.time()
    with tempfile.TemporaryDirectory() as tmpdir:
        generate_audio(
            text="Ready.",
            model=tts_model.get(),
            voice="af_heart",
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


@app.post("/speak")
async def speak(req: SpeakRequest):
    global last_request_time
    last_request_time = time.time()

    if not req.text.strip():
        raise HTTPException(status_code=400, detail="text must not be empty.")

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
                voice=req.voice,
                speed=req.speed,
                lang_code=lang_code,
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


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="127.0.0.1", port=8765)
