# Migration Plan: Whisper to Voxtral (MLX Client/Server)

**Status: complete.** Kept as the design record for *why* — see the
[README](README.md) and [`mlx-engine/README.md`](mlx-engine/README.md) for
current usage. The scope also grew past the original plan: the server now
serves TTS (`/speak`, Kokoro) alongside STT, and it's used beyond dictation
by the [Claude Code voice hooks](docs/claude-code-voice-hooks.md) and the
[menu bar app](ClaudeVoiceMenuBar/README.md).

This document originally outlined the plan to migrate `local-whisper` from
OpenAI's Whisper (via `whisper.cpp` subprocess) to Mistral AI's Voxtral using
a fast, `uv`-managed MLX inference server.

## Why Voxtral with MLX?

*   **Higher Accuracy:** ~4% WER vs ~10% for Whisper Large-v3.
*   **Semantic Understanding:** Built on a 4B parameter LLM, allowing for better handling of context and technical terms.
*   **Apple Silicon Optimization:** MLX is native to Mac and utilizes the unified memory architecture for high performance.
*   **Zero Cold Start:** By keeping the model warm in memory via a background server (started on demand, idles out after 15 minutes), dictation after the first request is fast.

## Technical Architecture (Client-Server)

Pivoted from a slow subprocess architecture (which suffered a 2-3 second cold boot per invocation) to a client-server model.

### 1. The Inference Server (`mlx-engine/`)
A dedicated Python sub-project managed by Astral's `uv`.
*   **Dependencies:** `fastapi`, `uvicorn`, `mlx-audio[tts]` (the `[tts]` extra pulls in STT support too).
*   **Function:** Loads Voxtral (STT) and Kokoro (TTS) independently and lazily — each on first use of its endpoint, not both at startup. Exposes `POST /transcribe` and `POST /speak`.
*   **Lifecycle:** Started on demand by the Go client / voice scripts via `scripts/voxtral-server.sh`; self-shuts-down after 15 minutes idle.

### 2. The Go Client (`local-whisper`)
The Go CLI acts as a thin client when the `--engine=voxtral` flag is used.
*   **Function:** Records audio via `sox`, then sends an HTTP POST request with the `.wav` file to `127.0.0.1:8765/transcribe`.

## Implementation Steps

### Phase 1: Research & Strategy — done
*   [x] Research Voxtral capabilities.
*   [x] Identify MLX as the optimal inference engine for Apple Silicon.
*   [x] Pivot to a Client-Server architecture managed by `uv` to solve cold-start latency.

### Phase 2: Server Implementation (`mlx-engine/`) — done
*   [x] Initialize a new `uv` project in `mlx-engine/` targeting Python 3.11+.
*   [x] Install `fastapi`, `uvicorn`, `python-multipart`, and `mlx-audio`.
*   [x] Write `mlx-engine/server.py` to load Voxtral and expose `/transcribe` — plus, beyond the original plan, Kokoro and `/speak`.
*   [x] `scripts/voxtral-server.sh` to start/stop/check status, with an atomic start-lock and idle-timeout shutdown.

### Phase 3: Client Implementation (Go) — done
*   [x] `pkg/voxtral/voxtral.go`: HTTP client posting audio to `/transcribe`, with proper error handling if the server is offline.
*   [x] `cmd/local-whisper/main.go` wires up `--engine=voxtral` alongside the original whisper.cpp path (`--engine=whisper`, still available).

### Phase 4: Validation & Testing — done
*   [x] Tested the `mlx-engine` server independently via `curl`.
*   [x] Verified Raycast and clipboard integration still work under `--engine=voxtral`.
*   [ ] Formal latency benchmark vs. Whisper — never produced hard numbers, informal use has been fast enough not to chase this further.

### Phase 5: Finalization — done
*   [x] Documentation updated (`README.md`, `mlx-engine/README.md`, and the two docs above).
*   [x] `scripts/setup-deps.sh` installs `uv`.

### Known limitation: `.whisper-context` doesn't work under `--engine=voxtral`
Not a missing wire-up — the model itself can't take it. `mlx-community/Voxtral-Mini-4B-Realtime-2602-4bit`
is `mlx_audio`'s `voxtral_realtime` implementation, whose `generate()` has no
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

---
*Last updated: 2026-08-29*
