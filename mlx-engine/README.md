# mlx-engine

Local speech server for Apple Silicon: speech-to-text (Voxtral) and
text-to-speech (Kokoro), both running on-device via [MLX](https://github.com/ml-explore/mlx).
No audio or text leaves the machine.

Used by `local-whisper` (`--engine=voxtral`) for dictation and by the
Claude Code voice hooks / menu bar app (`../scripts/speak.sh`,
`../ClaudeVoiceMenuBar/`) for spoken notifications — but it's a standalone
HTTP server, usable from anything that can `curl` `localhost:8765`.

## Run it

```bash
uv sync
uv run uvicorn server:app --host 127.0.0.1 --port 8765
```

Or via the parent project's process manager, which also handles PID
tracking and idle shutdown:

```bash
../scripts/voxtral-server.sh start   # or stop / status
```

Both models load **lazily**, on first request to their respective endpoint
— starting the server is instant, and a pure-TTS call doesn't pay for
loading the STT model (or vice versa). The server shuts itself down after
15 minutes of no `/transcribe` or `/speak` activity to free the RAM (`/health`
polls don't count as activity, so a monitoring UI polling every few seconds
won't keep it pinned open).

## Endpoints

| Endpoint | Method | Purpose |
|---|---|---|
| `/health` | GET | `{"status", "stt_loaded", "tts_loaded", ...}` — doesn't trigger loading or reset the idle timer |
| `/transcribe` | POST | `multipart/form-data` with a 16kHz mono `.wav` file (`audio` field) + optional `language` query param → `{"text", "latency_sec"}` |
| `/speak` | POST | JSON `{"text", "voice", "speed"}` → raw `audio/wav` bytes |

```bash
curl -X POST http://127.0.0.1:8765/speak \
  -H "Content-Type: application/json" \
  -d '{"text": "Hello from the local voice server.", "voice": "af_heart", "speed": 1.0}' \
  -o out.wav
```

## Models

- **STT**: `mlx-community/Voxtral-Mini-4B-Realtime-2602-4bit` — a 4B-parameter
  model, ~4% WER vs. ~10% for Whisper Large-v3, with better handling of
  technical vocabulary than a pure acoustic model.
- **TTS**: `mlx-community/Kokoro-82M-bf16` — small and fast, all English
  voices ship in the same download (`af_*`/`am_*`/`bf_*`/`bm_*`; see
  `../ClaudeVoiceMenuBar/` for the full list). Text-to-speech via
  Mistral's own [Voxtral-4B-TTS](https://huggingface.co/mistralai/Voxtral-4B-TTS-2603)
  is also available in `mlx-audio` (`voxtral_tts` model type) as a heavier,
  more expressive alternative if Kokoro's quality isn't enough for a given use case.

Both come from [`mlx-audio`](https://github.com/Blaizzy/mlx-audio), which
bundles a considerably longer list of STT/TTS models beyond what this
server currently wires up — worth a look if a different accuracy/speed/voice
tradeoff is ever needed.

## Dependencies

Managed by [`uv`](https://docs.astral.sh/uv/) — see `pyproject.toml`. Key
ones: `fastapi`, `uvicorn`, `mlx-audio[tts]` (Kokoro needs `misaki` for
phonemization, pulled in by the `[tts]` extra).

## Lint & format

```bash
uv run ruff check .    # lint
uv run ruff format .   # format, in place
```

Or from the repo root: `make lint` / `make fmt` (covers this and `voxtral/`
together with the Go side).
