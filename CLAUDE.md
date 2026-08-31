# CLAUDE.md - Project Instructions

Several components in this repo: the `local-whisper` Go CLI (dictation, see
below), `cmd/voice-monitor` (realtime call-transcript monitor, see
`SETUP.md`), `mlx-engine/` (Python STT+TTS server used by `local-whisper`),
`voxtral/` (Python/MLX primitives used by `voice-monitor`), `scripts/`
(Claude Code voice hooks), and `ClaudeVoiceMenuBar/` (Swift menu bar app).
See each one's own README/SETUP for build/run instructions specific to it —
this file covers the Go side.

## Build & Run

```bash
make build              # Build local-whisper binary to bin/local-whisper
make build-voice-monitor # Build the realtime call-transcript monitor (see SETUP.md)
make test                # Run all tests (go test -v ./... + scripts/voice_hooks pytest)
make vet                  # go vet ./...
make fmt                  # gofmt + ruff format (mlx-engine/, voxtral/, scripts/voice_hooks/), in place
make fmt-check            # Same, but check-only — no writes (CI-safe)
make lint                  # vet + fmt-check + ruff check (mlx-engine/, voxtral/, scripts/voice_hooks/)
make clean                # Remove bin/
make start-engine          # Start mlx-engine (needed for local-whisper -engine voxtral)
make setup-voice-hooks     # Create the venv for scripts/voice_hooks/ (needed before hook-stop.sh/hook-notify.sh can summarize/truncate)
```

Run `make lint` before committing. The Python components (`mlx-engine/`, `voxtral/`, `scripts/voice_hooks/`) are linted/formatted with `ruff` (a `uv` dev dependency in each's `pyproject.toml`); `E501` is intentionally off there since `ruff format` governs code line length and the rest is unwrappable help/print strings.

## Project Structure

- `cmd/local-whisper/` - CLI entry point (dictation)
- `cmd/voice-monitor/` - realtime transcript monitor: serves a live transcript over SSE at localhost and logs it to a file; input can be the mic or a loopback device (e.g. BlackHole) for capturing call audio
- `internal/recording/` - Audio recording via sox, including peak normalization (`norm -3`) as part of the same sox invocation — capture already happens at `internal/audio`'s target rate/channels, so there's no separate resample/normalize pass
- `internal/audio/` - shared sox target format constants (`SampleRateHz`, `Channels`) that recording and the transcription engines must agree on
- `internal/clipboard/` - macOS clipboard + paste via AppleScript
- `pkg/transcribe/` - the `Options`/`Client` shapes shared by the one-shot transcription engines below, so `cmd/local-whisper` can pick one at runtime without branching on engine-specific types
- `pkg/whisper/` - whisper-cli subprocess wrapper (`local-whisper -engine whisper`, default)
- `pkg/mlxengine/` - HTTP client for `mlx-engine/` (`local-whisper -engine voxtral`)
- `pkg/voxtral/` - a *different*, independent client: wraps the `voxtral/` Python/MLX primitives directly via `os/exec`, used only by `cmd/voice-monitor`. Same underlying model family as `mlx-engine`, different local architecture — don't confuse the two.
- `mlx-engine/` - local STT/TTS server for `local-whisper` (Python, `uv`-managed — see `mlx-engine/README.md`)
- `voxtral/` - Python/MLX primitives for `voice-monitor` (uv project): Voxtral STT (Mini 3B, Mini 4B Realtime), a Whisper fallback engine (multilingual, for languages Voxtral doesn't cover), and Sortformer speaker diarization; see `make setup-voxtral` and `SETUP.md`
- `scripts/` - dependency/model setup, plus the Claude Code voice hooks (see `docs/claude-code-voice-hooks.md`); `scripts/voice_hooks/` is its own `uv` project (flat scripts, no nested package, matching `voxtral/`'s pattern) holding the hooks' text processing (markdown stripping, sentence-aware truncation, Ollama summarization) — `hook-stop.sh`/`hook-notify.sh` stay thin bash entry points that shell out to it once per firing
- `ClaudeVoiceMenuBar/` - menu bar app for tuning voice settings (Swift, see its own README)

## Conventions

- **stdlib only** - No third-party Go dependencies. `voxtral/` is external Python/MLX tooling shelled out to via `os/exec`, same as whisper-cli/sox — not a Go dependency. (The Python/Swift components have their own dependency managers — `uv` and SwiftPM respectively.)
- **macOS-specific** - Uses `afplay`, `pbcopy`, AppleScript, sox, whisper-cli, MLX (Apple Silicon only)
- **Error handling** - Return errors up; `fmt.Fprintf(os.Stderr, ...)` + `os.Exit(1)` at top level
- **Resource cleanup** - Always check `os.Open`/HTTP response errors and `defer Close()`
- **Temp files** - Written to `/tmp/voice-input/`, cleaned up on exit
- **Ports** - `mlx-engine` binds `127.0.0.1:8765` — don't reuse this port for anything else in this repo; it was previously 8000, which is common enough for other local dev servers (Django, Docker port mappings, etc.) to collide with
