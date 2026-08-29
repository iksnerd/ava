# Migration Plan: Whisper to Voxtral (MLX Client/Server)

This document outlines the plan to migrate `local-whisper` from OpenAI's Whisper (via `whisper.cpp` subprocess) to Mistral AI's Voxtral using a fast, `uv`-managed MLX inference server.

## Why Voxtral with MLX?

*   **Higher Accuracy:** ~4% WER vs ~10% for Whisper Large-v3.
*   **Semantic Understanding:** Built on a 4B parameter LLM, allowing for better handling of context and technical terms.
*   **Apple Silicon Optimization:** MLX is native to Mac and utilizes the unified memory architecture for high performance.
*   **Zero Cold Start:** By keeping the 2.4GB model warm in memory via a background server, we achieve true real-time dictation (latency < 100ms).

## Technical Architecture (Client-Server)

We are pivoting from a slow subprocess architecture (which suffers a 2-3 second cold boot) to a fast client-server model.

### 1. The Inference Server (`mlx-engine/`)
A dedicated Python sub-project managed by Astral's `uv`.
*   **Dependencies:** `fastapi`, `uvicorn`, `mlx-audio[stt]`.
*   **Function:** Loads `Voxtral-Mini-4B-Realtime-4bit` into RAM once on startup. Exposes a `POST /transcribe` endpoint.
*   **Lifecycle:** Must be started in the background before dictation can occur.

### 2. The Go Client (`local-whisper`)
Our existing Go CLI acts as a thin client when the `--engine=voxtral` flag is used.
*   **Function:** Records audio via `sox`, then sends an HTTP POST request with the `.wav` file to `localhost:8000/transcribe`.

## Implementation Steps

### Phase 1: Research & Strategy (Completed)
*   [x] Research Voxtral capabilities.
*   [x] Identify MLX as the optimal inference engine for Apple Silicon.
*   [x] Pivot to a Client-Server architecture managed by `uv` to solve cold-start latency.

### Phase 2: Server Implementation (`mlx-engine/`)
*   [ ] Initialize a new `uv` project in `mlx-engine/` targeting Python 3.11+.
*   [ ] Install `fastapi`, `uvicorn`, `python-multipart`, and `mlx-audio[stt]`.
*   [ ] Write `mlx-engine/main.py` to load the Voxtral model and expose the `/transcribe` endpoint.
*   [ ] Create a `Makefile` or script to easily start/stop the server.

### Phase 3: Client Implementation (Go)
*   [ ] Refactor `pkg/voxtral/voxtral.go`:
    *   Remove subprocess logic.
    *   Implement HTTP client logic to POST audio to `http://localhost:8000/transcribe`.
    *   Add error handling if the server is offline.

### Phase 4: Validation & Testing
*   [ ] Test the `mlx-engine` server independently using `curl`.
*   [ ] Benchmark end-to-end latency (recording -> transcription text) vs Whisper.
*   [ ] Verify Raycast and Clipboard integration still works.

### Phase 5: Finalization
*   [ ] Update documentation (`README.md`) with instructions on how to run the background server.
*   [ ] Update `scripts/setup-deps.sh` to install `uv` instead of a standard python `venv`.

---
*Last updated: April 1, 2026*
