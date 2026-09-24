# CLAUDE.md - Project Instructions

Several components in this repo: the `ava` Go CLI (dictation, see
below), `cmd/ava-monitor` (realtime call-transcript monitor, see
`docs/ava-monitor.md`), `mlx-engine/` (Python Kokoro TTS server used by `ava`),
`voxtral/` (Python/MLX primitives used by `ava-monitor`), `scripts/`
(Claude Code voice hooks), and `AvaMenuBar/` (Swift menu bar app).
See each one's own README/SETUP for build/run instructions specific to it —
this file covers the Go side. The same voice stack is also served to MCP
clients by `ava mcp` (see `docs/mcp.md`).

**Which engine does what**: `whisper.cpp` transcribes and Kokoro speaks, and
that is the whole of `ava` — nothing to download beyond `make
setup-model`/`make setup-deps`. There is no `--engine` flag any more: the
Voxtral STT path through `mlx-engine` was removed after it measured 13x slower
than whisper.cpp on the same audio for a near-identical transcript.

Voxtral is still the default engine of `cmd/ava-monitor`, which is a different
problem: streaming a live transcript, where `pkg/stt/whisper` transcribes a
complete file per subprocess. That lives in `voxtral/`, is opt-in via `make setup-voxtral`, and downloads
its 2.9GB model on first real use. Don't assume it is available.

Start/stop/check the mlx-engine (Kokoro) server with `ava engine
start`/`stop`/`status`, or the equivalent `make start-engine`/`stop-engine`/
`status-engine` targets.

## Build & Run

```bash
make build              # Build ava binary to bin/ava
make build-ava-monitor # Build the realtime call-transcript monitor (see docs/ava-monitor.md)
make test                # Run all tests (go test -v ./... + scripts/voice_hooks pytest + test-mlx-engine + test-voxtral)
make vet                  # go vet ./...
make fmt                  # gofmt + ruff format (mlx-engine/, voxtral/, scripts/voice_hooks/), in place
make fmt-check            # Same, but check-only — no writes (CI-safe)
make lint                  # vet + fmt-check + check-paths/names/protocol/enginedist/docs + ruff check (mlx-engine/, voxtral/, scripts/voice_hooks/)
make clean                # Remove bin/
make start-engine          # Start mlx-engine (Kokoro TTS server)
make setup-voice-hooks     # Create the venv for scripts/voice_hooks/ (needed before hook-stop.sh/hook-notify.sh can summarize/truncate)
```

To smoke-test mlx-engine for real (not the stubbed `make test-mlx-engine`), run it from a short path such as the repo's own `mlx-engine/` on a spare `--port`: espeak-ng truncates its data path at 160 bytes, so a venv under a long temp or scratch directory dies with a missing `phontab` that looks like a dependency bug (see `checkPathBudget` in `cmd/ava/setup.go`).

To smoke-test speech for real without hearing it or touching your settings, point both config readers at a temp file and zero the volume: `VOICE_CONFIG_FILE=$f VOICECONFIG_PATH=$f TTS_VOLUME=0 bash scripts/speak.sh "..."` with `$f` holding `{"muted": false}`. A machine left muted otherwise turns every speak into a silent no-op that looks like a hand-off bug.

Run `make lint` before committing. The Python components (`mlx-engine/`, `voxtral/`, `scripts/voice_hooks/`) are linted/formatted with `ruff` (a `uv` dev dependency in each's `pyproject.toml`); `E501` is intentionally off there since `ruff format` governs code line length and the rest is unwrappable help/print strings.

## Project Structure

- `cmd/ava/` - CLI entry point. The bare command dictates (record → transcribe → paste); `speak.go`, `stop.go`, `voices.go`, `transcribe.go` and `a11y.go` expose the rest of the voice stack as verbs, and `mcp.go` serves the same five capabilities to MCP clients over stdio (see `docs/mcp.md`). Keep the two surfaces in step: a capability reachable from one and not the other is the gap this layout exists to prevent. Nothing in the `mcp` command may write to stdout — it's the JSON-RPC channel. `setup.go` and `setup_model.go` install what the binary needs without a checkout (sox, whisper-cli, the pinned and sha256-verified model, the Kokoro engine bundle), and `engine.go` starts, stops and checks the Kokoro server.
- `cmd/ava-monitor/` - realtime transcript monitor: serves a live transcript over SSE at localhost and logs it to a file; input can be the mic or a loopback device (e.g. BlackHole) for capturing call audio
- `internal/recording/` - Audio recording via sox, including peak normalization (`norm -3`) as part of the same sox invocation — capture already happens at `internal/audio`'s target rate/channels, so there's no separate resample/normalize pass
- `internal/audio/` - shared sox target format constants (`SampleRateHz`, `Channels`) that recording and the transcription engines must agree on
- `internal/clipboard/` - macOS clipboard + paste via AppleScript
- `internal/voiceconfig/` - the live Ava settings (mute, speed, volume, voice, `say` rate, engine auto-start). A hand-written Go port of `scripts/lib.sh`'s `config_get`/`config_get_bool`, because an installed binary can't reach the repo's `scripts/`. Default *values* are pinned to `scripts/voice-defaults.json` by a test, and the voice list to `VoiceSettings.swift` by another — `go:embed` can't reach a parent directory and a second copy of either file would defeat its single-source-of-truth job. **Three readers resolve this one config file** (bash, for the hooks' mute and engine auto-start only; Go; Swift) and what diverges is the resolution *logic*, not the values: `contract_test.go` runs bash and Go against the shared cases in `testdata/voice-config-cases.json` and fails if they disagree; `make check-swift-config` is the Swift arm, kept out of `make test` because it needs swiftc. Add a case there before changing any reader
- `internal/speaker/` - synthesis + playback in Go, and the only speak implementation: `scripts/speak.sh` (hooks, menu bar) is a mute check plus a hand-off to `ava speak --async`. It had its own player once, and a stop fix made to one missed the other. Checks the global mute first, keeps an activity marker in `/tmp/ava-tts-active` (what the menu bar's indicator polls and `internal/ttscontrol` cancels), holds a `flock(2)` on `/tmp/ava-tts-playback.lock` so concurrent speaks queue, and falls back to `say` when the server is down. Writes no `.synth.pid`: synthesis here is an in-process HTTP call, so naming our own PID would have a stop kill the whole process
- `internal/ttsproto/` and `internal/protocol/` - the marker-file protocol every speak and stop path shares (activity dir, playback lock, port). `protocol` is generated from `protocol.json` into Go, bash (`scripts/protocol.sh`) and Swift; `ttsproto` re-exports it under the names the Go packages use. `make check-protocol` fails on a stale copy
- `internal/ttscontrol/` - cancels speech in flight before the mic opens; a Go port of `scripts/stop-speaking.sh`'s marker-file protocol, since an installed binary can't reach the script
- `internal/enginedist/` - the mlx-engine bundle embedded in the binary, so `ava setup` can install Kokoro TTS without a checkout. `make check-enginedist` fails when the embedded copy is stale
- `internal/buildinfo/` - which build is running, for `--version` and the MCP handshake
- `internal/procutil/` - small subprocess helpers shared by the CLIs and engine clients
- `internal/testutil/` - helpers shared by the test suites (a regular package so any `_test.go` can import it)
- `internal/a11y/` - parses chrome-devtools MCP's `take_snapshot` accessibility tree and renders it as screen-reader announcements, plus the findings that only surface when a page is heard in order. Pure functions, no I/O — the rendering has to be identical run to run for a spoken audit to mean anything
- `pkg/stt/` - the `Options`/`Client` shapes of the one-shot transcription engine. Every STT engine lives under here as its own subpackage; there's no `pkg/tts` — synthesis is `pkg/mlx.Client.Speak` wrapped by `internal/speaker` (see `pkg/stt`'s own doc comment)
- `pkg/stt/whisper/` - whisper-cli subprocess wrapper; the only transcription engine `ava` has
- `pkg/mlx/` - HTTP client for `mlx-engine/`'s Kokoro TTS. Lives at the top level rather than under `pkg/stt/` because it is not an STT client: mlx-engine's STT half was removed after whisper.cpp measured 13x faster on the same audio
- `pkg/stt/realtime/` - a *different*, independent client from `pkg/mlx`: wraps `voxtral/realtime.py` directly via `os/exec`, used only by `cmd/ava-monitor`. Streaming Voxtral, where `mlx-engine` serves only Kokoro TTS.
- `mlx-engine/` - local Kokoro TTS server for `ava` (Python, `uv`-managed — see `mlx-engine/README.md`)
- `voxtral/` - Python/MLX primitives for `ava-monitor` (uv project): Voxtral STT (Mini 4B Realtime), a Whisper fallback engine (multilingual, for languages Voxtral doesn't cover), and Sortformer speaker diarization; see `make setup-voxtral` and `docs/ava-monitor.md`
- `scripts/` - dependency/model setup, plus the Claude Code voice hooks (see `docs/claude-code-voice-hooks.md`); `scripts/voice_hooks/` is its own `uv` project (flat scripts, no nested package, matching `voxtral/`'s pattern) holding the hooks' text processing (markdown stripping, sentence-aware truncation, Ollama summarization) — `hook-stop.sh`/`hook-notify.sh` stay thin bash entry points that shell out to it once per firing
- `AvaMenuBar/` - menu bar app for tuning voice settings (Swift, see its own README)

## Conventions

- **Minimal Go dependencies** - two third-party Go dependencies, both load-bearing: `github.com/spf13/cobra` (CLI command/flag framework), used by both `cmd/ava` and `cmd/ava-monitor` for their command trees, and `github.com/modelcontextprotocol/go-sdk` (used only by `cmd/ava/mcp.go`) — hand-rolling JSON-RPC-over-stdio against a spec that still moves would cost more than the dependency does. Don't add a third without the same kind of reason. `voxtral/` is external Python/MLX tooling shelled out to via `os/exec`, same as whisper-cli/sox — not a Go dependency. (The Python/Swift components have their own dependency managers — `uv` and SwiftPM respectively.) Each binary's cobra wiring lives in its own `cmd/<binary>/` directory: `main.go` just calls `Execute()`, `root.go` builds the root command, and each subcommand (e.g. `engine.go`, `devices.go`) is its own file with a `newXCmd()` constructor.
- **macOS-specific** - Uses `afplay`, `pbcopy`, AppleScript, sox, whisper-cli, MLX (Apple Silicon only)
- **Error handling** - Return errors up; `fmt.Fprintf(os.Stderr, ...)` + `os.Exit(1)` at top level
- **Resource cleanup** - Always check `os.Open`/HTTP response errors and `defer Close()`
- **Temp files** - Written to `/tmp/voice-input/`, cleaned up on exit
- **Ports** - `mlx-engine` binds `127.0.0.1:8765`; `ava-monitor` serves its live transcript on `8766`. Don't reuse either for anything else in this repo, and don't go back to 8000, which is common enough for other local dev servers (Django, Docker port mappings, etc.) to collide with. `cmd/ava-monitor` defaulted to 8765 until it was caught by a test — the two never collided in normal use only because nobody ran a call monitor and dictation in the same minute
