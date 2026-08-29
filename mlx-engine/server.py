import io
import time
import os
import tempfile
import asyncio
import sys
from fastapi import FastAPI, UploadFile, File, HTTPException
from fastapi.responses import Response
from pydantic import BaseModel
import mlx.core as mx
from mlx_audio.stt.generate import generate_transcription
from mlx_audio.utils import load_model
from mlx_audio.tts.generate import generate_audio
from mlx_audio.tts.utils import load_model as load_tts_model

app = FastAPI(title="Local Voice MLX Inference Server")

MODEL_PATH = "mlx-community/Voxtral-Mini-4B-Realtime-2602-4bit"
TTS_MODEL_PATH = "mlx-community/Kokoro-82M-bf16"
last_request_time = time.time()
IDLE_TIMEOUT_SEC = 15 * 60  # 15 minutes


class SpeakRequest(BaseModel):
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
                print(f"{self.label} model loaded successfully in {time.time() - start_time:.2f} seconds.")
            except Exception as e:
                raise HTTPException(status_code=500, detail=f"Failed to load {self.label} model: {e}")
        return self.instance

    @property
    def loaded(self) -> bool:
        return self.instance is not None


stt_model = LazyModel("STT", MODEL_PATH, load_model)
tts_model = LazyModel("TTS", TTS_MODEL_PATH, load_tts_model)

async def idle_shutdown_checker():
    """Background task that shuts down the server after a period of inactivity."""
    while True:
        await asyncio.sleep(60)  # Check every minute
        idle_time = time.time() - last_request_time
        if idle_time > IDLE_TIMEOUT_SEC:
            print(f"Server idle for {IDLE_TIMEOUT_SEC/60:.0f} minutes. Shutting down to free RAM...")
            PID_FILE = "/tmp/voxtral-server.pid"
            if os.path.exists(PID_FILE):
                try:
                    os.remove(PID_FILE)
                except:
                    pass
            os._exit(0)

@app.on_event("startup")
async def startup_event():
    # Both models are loaded lazily on first use of their respective endpoint,
    # so starting the server (and a cold /speak call) doesn't have to pay for
    # loading the 4B Voxtral STT model when only TTS is needed, and vice versa.
    asyncio.create_task(idle_shutdown_checker())

@app.get("/health")
async def health_check():
    # Deliberately does NOT touch last_request_time: the menu bar app polls
    # this every few seconds, and doing so would defeat the idle-shutdown
    # timer for as long as it's open. Only real work (/transcribe, /speak)
    # counts as activity.
    return {
        "status": "ok",
        "model": MODEL_PATH,
        "stt_loaded": stt_model.loaded,
        "tts_model": TTS_MODEL_PATH,
        "tts_loaded": tts_model.loaded,
    }

@app.post("/transcribe")
async def transcribe(audio: UploadFile = File(...), language: str = "en"):
    global last_request_time
    last_request_time = time.time()

    model_instance = stt_model.get()

    if not audio.filename.endswith(".wav"):
        raise HTTPException(status_code=400, detail="Only .wav files are supported.")

    start_time = time.time()
    try:
        # 1. Read the uploaded file directly into an in-memory byte buffer
        audio_bytes = await audio.read()
        buf = io.BytesIO(audio_bytes)
        
        import soundfile as sf
        import numpy as np
        
        # 2. Decode the WAV file from memory
        # We assume 16kHz mono (which the Go client enforces via sox)
        audio_data, samplerate = sf.read(buf, dtype="float32")
        
        # If stereo, convert to mono by averaging channels
        if len(audio_data.shape) > 1:
            audio_data = np.mean(audio_data, axis=1)

        # 3. Convert to MLX array directly to bypass disk I/O
        audio_array = mx.array(audio_data)

        try:
            # 4. Generate transcription using the loaded model and in-memory array
            # We set `beam_size=1` for faster greedy decoding, and disable verbose logs
            result = generate_transcription(
                model=model_instance,
                audio=audio_array,
                language=language,
                temp=0.0,
                beam_size=1,
                verbose=False
            )

            # Extract the actual text from the STTOutput object or other formats
            if hasattr(result, "text"):
                final_text = result.text
            elif isinstance(result, list):
                final_text = " ".join([seg.get("text", "") for seg in result if isinstance(seg, dict)])
            elif isinstance(result, dict):
                final_text = result.get("text", "")
            else:
                final_text = str(result)

            return {
                "text": final_text.strip(),
                "latency_sec": round(time.time() - start_time, 3)
            }
        except Exception as inner_e:
            raise HTTPException(status_code=500, detail=f"Generation failed: {inner_e}")
            
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

@app.post("/speak")
async def speak(req: SpeakRequest):
    global last_request_time
    last_request_time = time.time()

    if not req.text.strip():
        raise HTTPException(status_code=400, detail="text must not be empty.")

    tts_model_instance = tts_model.get()

    with tempfile.TemporaryDirectory() as tmpdir:
        prefix = "speech"
        try:
            generate_audio(
                text=req.text,
                model=tts_model_instance,
                voice=req.voice,
                speed=req.speed,
                output_path=tmpdir,
                file_prefix=prefix,
                audio_format="wav",
                save=True,
                play=False,
                verbose=False,
            )
        except Exception as e:
            raise HTTPException(status_code=500, detail=f"Speech generation failed: {e}")

        wav_path = os.path.join(tmpdir, f"{prefix}_000.wav")
        if not os.path.exists(wav_path):
            raise HTTPException(status_code=500, detail="Speech generation produced no audio.")
        with open(wav_path, "rb") as f:
            audio_bytes = f.read()

    return Response(content=audio_bytes, media_type="audio/wav")


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="127.0.0.1", port=8765)
