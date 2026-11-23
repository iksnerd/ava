# mlx-engine

Local text-to-speech server for Apple Silicon: Kokoro, running on-device via
[MLX](https://github.com/ml-explore/mlx). No text or audio leaves the machine.

Used by `ava speak`, the Claude Code voice hooks, the MCP `speak` tool
and the Ava menu bar app (`../scripts/speak.sh`, `../AvaMenuBar/`), but it's a
standalone HTTP server, usable from anything that can `curl` `localhost:8765`.

## Run it

```bash
uv sync
uv run uvicorn server:app --host 127.0.0.1 --port 8765
```

Or via the parent project's process manager, which also handles PID
tracking and idle shutdown:

```bash
../scripts/mlx-engine-server.sh start   # or stop / status
```

Kokoro is loaded and warmed at startup, so a healthy server is a ready one:
its first `generate_audio` call costs an extra ~2.6s of MLX compilation, paid
once at start rather than on whoever speaks first. On an M3 Pro,
every real `/speak` after that stays at Kokoro's steady-state ~230-290ms.
`/health` won't respond until the warmup finishes. The server shuts itself down after 15 minutes of no
`/speak` activity to free the RAM (`/health` polls don't
count as activity, so a monitoring UI polling every few seconds won't keep
it pinned open); the next `/speak` after that pays the warmup again.

## Endpoints

| Endpoint | Method | Purpose |
|---|---|---|
| `/health` | GET | `{"status", "protocol", "tts_model", "tts_loaded"}`; doesn't trigger loading or reset the idle timer |
| `/speak` | POST | JSON `{"text", "voice", "speed", "split_pattern"}` → raw `audio/wav` bytes |
| `/speak/timed` | POST | Same request as `/speak`; returns the WAV plus where each segment sits in it |

Every field, the response shapes and the errors are in [docs/engine-api.md](../docs/engine-api.md).

```bash
curl -X POST http://127.0.0.1:8765/speak \
  -H "Content-Type: application/json" \
  -d '{"text": "Hello from the local voice server.", "voice": "af_heart", "speed": 1.0}' \
  -o out.wav
```

### Getting more out of Kokoro

No model swap needed for any of these; they're request-level knobs Kokoro
(via `mlx-audio`) already supports:

- **Correct accent per voice**: `lang_code` is derived server-side from
  `voice`'s first letter (`af_`/`am_` → American English, `bf_`/`bm_` →
  British English, and so on for Kokoro's other-locale voices), so a
  British voice actually gets phonemized with British rules instead of
  defaulting to American English.
- **Voice blending**: pass a comma-separated `voice`, e.g.
    `"af_heart,af_sky"`: Kokoro averages the style vectors, and it is
    not limited to two: any number of comma-separated ids works.
- **Per-word pronunciation/stress overrides**: inline markdown-link-style
  markup in `text`: `` [Kokoro](/kˈOkəɹO/) `` for explicit IPA, or an integer
  like `` [word](-1) `` to shift stress (see
  [`../docs/tuning.md`](../docs/tuning.md#intonation-and-stress)),
  useful for names, acronyms, or jargon Kokoro would otherwise mispronounce.

## Models

This server is TTS-only. One-shot transcription is `pkg/stt/whisper`'s job
(whisper.cpp: 1.26s on a 20s sample, against 17s warm and 127s cold for
Voxtral), and `internal/monitor` runs Voxtral in its own process for streaming.

`mlx_audio` bundles a considerably longer list of STT/TTS models beyond what this
server currently wires up; worth a look if a different accuracy/speed/voice
tradeoff is ever needed.

## Dependencies

Managed by [`uv`](https://docs.astral.sh/uv/); see `pyproject.toml`. Key
ones: `fastapi`, `uvicorn`, `mlx-audio[tts]` (Kokoro needs `misaki` for
phonemization, pulled in by the `[tts]` extra).

## Lint & format

```bash
uv run ruff check .    # lint
uv run ruff format .   # format, in place
```

Or from the repo root: `make lint` / `make fmt` (covers this, `voxtral/` and
`scripts/voice_hooks/` together with the Go side).
