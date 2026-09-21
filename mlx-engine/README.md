# mlx-engine

Local speech server for Apple Silicon: speech-to-text (Voxtral) and
text-to-speech (Kokoro), both running on-device via [MLX](https://github.com/ml-explore/mlx).
No audio or text leaves the machine.

Used by `local-whisper` (`--engine=voxtral`) for dictation and by the
Claude Code voice hooks / menu bar app (`../scripts/speak.sh`,
`../AvaMenuBar/`) for spoken notifications — but it's a standalone
HTTP server, usable from anything that can `curl` `localhost:8765`.

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

The STT model loads **lazily**, on first `/transcribe` request, so starting
the server doesn't pay for the 4B Voxtral model when only TTS is needed.
TTS loads and warms up (a throwaway synthesis to pay MLX's one-time JIT
compile cost) eagerly at startup instead — measured on an M3 Pro, that
warmup takes a few seconds once, in exchange for every real `/speak`
request afterward staying at Kokoro's steady-state ~230-290ms instead of
one of them randomly paying an extra ~2.6s. `/health` won't respond until
that warmup finishes. The server shuts itself down after 15 minutes of no
`/transcribe` or `/speak` activity to free the RAM (`/health` polls don't
count as activity, so a monitoring UI polling every few seconds won't keep
it pinned open) — the next `/speak` after that pays the warmup again.

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

### Getting more out of Kokoro

No model swap needed for any of these — they're request-level knobs Kokoro
(via `mlx-audio`) already supports:

- **Correct accent per voice**: `lang_code` is derived server-side from
  `voice`'s first letter (`af_`/`am_` → American English, `bf_`/`bm_` →
  British English, and so on for Kokoro's other-locale voices) — so a
  British voice actually gets phonemized with British rules instead of
  defaulting to American English.
- **Voice blending**: pass a comma-separated `voice`, e.g.
  `"af_heart,af_sky"` — Kokoro averages the two voices' embeddings into a
  blended one.
- **Per-word pronunciation/stress overrides**: inline markdown-link-style
  markup in `text` — `` [Kokoro](/kˈOkəɹO/) `` for an explicit IPA
  pronunciation, `` [word](+0.5) `` / `` [word](-0.5) `` to shift stress —
  useful for names, acronyms, or jargon Kokoro would otherwise mispronounce.

## Models

- **STT**: `mlx-community/Voxtral-Mini-4B-Realtime-2602-4bit` — a 4B-parameter
  model, ~4% WER vs. ~10% for Whisper Large-v3, with better handling of
  technical vocabulary than a pure acoustic model.
- **TTS**: `mlx-community/Kokoro-82M-bf16` — small and fast, all English
  voices ship in the same download (`af_*`/`am_*`/`bf_*`/`bm_*`; see
  `../AvaMenuBar/` for the full list). Text-to-speech via
  Mistral's own [Voxtral-4B-TTS](https://huggingface.co/mistralai/Voxtral-4B-TTS-2603)
  is also available in `mlx-audio` (`voxtral_tts` model type) as a heavier,
  more expressive alternative if Kokoro's quality isn't enough for a given use case.

Both come from [`mlx-audio`](https://github.com/Blaizzy/mlx-audio), which
bundles a considerably longer list of STT/TTS models beyond what this
server currently wires up — worth a look if a different accuracy/speed/voice
tradeoff is ever needed.

### Known limitation: `.whisper-context` doesn't work under `--engine voxtral`

Not a missing wire-up — the model itself can't take it.
`mlx-community/Voxtral-Mini-4B-Realtime-2602-4bit` is `mlx_audio`'s
`voxtral_realtime` implementation, whose `generate()` has no
prompt/context parameter at all: its input to the decoder is a hardcoded
`[BOS] + [STREAMING_PAD]*n + audio embeddings` sequence with no slot for
injected text (checked the source directly, not just the public signature).

`mlx_audio` does ship a *different*, non-realtime Voxtral model
(`voxtral`, not `voxtral_realtime`) whose `generate()` takes a chat-style
`message` list that could carry a context hint — but swapping to it means
running a second, slower model path just for this, working against the
whole reason Voxtral-Realtime was chosen (fast, always-warm dictation).
Decided to leave `.whisper-context` as whisper-engine-only rather than
take that tradeoff.

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
