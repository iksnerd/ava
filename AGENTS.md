# local-whisper Agent Guide

This repo is six components: the `local-whisper` Go CLI (dictation),
`cmd/voice-monitor` (realtime call-transcript monitor), the `mlx-engine`
Python server (local STT+TTS, used by the CLI and by everything below),
`voxtral/` (Python/MLX primitives used by `voice-monitor`), `scripts/`
(Claude Code voice hooks), and `ClaudeVoiceMenuBar` (a Swift menu bar app
for tuning voice settings). This guide covers the Go CLI (`local-whisper`)
specifically; see each other component's own docs — `SETUP.md`
(`voice-monitor`), `mlx-engine/README.md`, `docs/claude-code-voice-hooks.md`,
`ClaudeVoiceMenuBar/README.md`.

## Build Commands
- `make build` - Compile binary to `bin/local-whisper`
- `make test` - Run all Go tests plus the `scripts/voice_hooks/` pytest suite
- `make setup-model` - Download Whisper model to ~/.local/share/whisper-cpp/
- `make install-raycast` - Build, download model, install Raycast command
- `make install-bin` - Build, download model, install to ~/.local/bin
- `make start-engine` / `stop-engine` / `status-engine` - Manage the `mlx-engine` background server (needed for `--engine voxtral`); thin wrappers around `local-whisper engine start` / `stop` / `status`
- `make clean` - Remove bin/ directory
- `go run ./cmd/local-whisper [flags]` - Run directly without building

## Architecture
Multi-package CLI tool for local voice transcription, with two interchangeable
engines: `whisper.cpp` via subprocess (default, zero extra setup), or Voxtral
via a local HTTP server (`--engine voxtral`, see `pkg/mlx/` and
`mlx-engine/` — advanced/opt-in, needs `make setup-voxtral`). Both engines
satisfy the shared `pkg/stt.Client` interface so `cmd/local-whisper` picks
one at runtime without branching on engine-specific types.

**Project Structure:**
```
cmd/local-whisper/main.go       - func main() { Execute() }
cmd/local-whisper/root.go       - cobra root command: flags, RunE (record → transcribe → output)
cmd/local-whisper/engine.go     - `engine` subcommand group (start/stop/status), wraps
                                  scripts/mlx-engine-server.sh
cmd/local-whisper/model.go      - engine/model flag validation, model filename selection
cmd/local-whisper/context.go    - .whisper-context loading
cmd/local-whisper/dependencies.go - external dependency checks (sox, whisper-cli, mlx-engine health)
internal/recording/recorder.go  - Audio recording, silence detection, and peak
                                  normalization (norm -3) in one sox invocation
internal/audio/audio.go         - shared sox format constants (SampleRateHz, Channels)
internal/clipboard/clipboard.go - Clipboard & auto-paste operations
internal/procutil/              - shared subprocess/signal helpers
pkg/stt/stt.go                  - Options/Client shapes shared by the two engines below,
                                  plus WriteOutputIfRequested() they both call
pkg/stt/whisper/whisper.go      - whisper-cli subprocess wrapper (--engine whisper)
pkg/mlx/mlx.go                  - mlx-engine HTTP client (--engine voxtral) — lives at
                                  the top level, not nested under pkg/stt, since
                                  mlx-engine itself serves TTS as much as STT
mlx-engine/                     - the local STT/TTS server (separate Python/uv project)
scripts/setup-model.sh          - Auto-download whisper model script
scripts/mlx-engine-server.sh    - Start/stop/status for mlx-engine, wrapped by `local-whisper engine`
```

Not covered here: `pkg/stt/realtime` (a *different*, independent client —
wraps `voxtral/realtime.py` via `os/exec`, used only by `cmd/voice-monitor`,
same underlying model family as `mlx-engine` but a different local
architecture) and `voxtral/` itself. See `SETUP.md`. There is no `pkg/tts` —
nothing in this repo speaks Go to a TTS engine directly; all synthesis goes
through `scripts/speak.sh` curling `mlx-engine`'s `/speak` (see `pkg/stt`'s
own package doc comment for why).

**External dependencies** (not in go.mod):
- `whisper-cli` - OpenAI Whisper C++ implementation (`--engine whisper`, the default)
- `sox` - Audio recording with silence detection
- `afplay` - Sound playback (macOS, async)
- `osascript` - AppleScript for auto-paste (macOS)
- `uv` - runs `mlx-engine`, needed for `--engine voxtral` (start it with `local-whisper engine start`, which shells out to `scripts/mlx-engine-server.sh`)

**Model location** (whisper engine): `~/.local/share/whisper-cpp/ggml-base.en.bin` (141MB, auto-downloaded by `make setup-model`)
**Alternate model**: `ggml-tiny.en.bin` (74MB, faster but less accurate)

**Voxtral engine**: `mlx-community/Voxtral-Mini-4B-Realtime-2602-4bit`, served by `mlx-engine/server.py` at `127.0.0.1:8765`, loaded lazily on first `/transcribe` request. See `mlx-engine/README.md`.

**Temp directory**: `/tmp/voice-input/` (raw/processed WAV files, transcript)

**Recording behavior**:
- Starts immediately while Blow.aiff plays in background
- Stops after 2 seconds of silence at 3% threshold
- Normalizes audio before transcription

## Code Style
- **Imports**: `github.com/spf13/cobra` (and its `pflag` dependency) is the only external Go dependency, used for the CLI command tree; everything else is stdlib
- **Naming**: CamelCase for functions; descriptive names (e.g., `recordAudio`, `pasteWithAppleScript`)
- **Error handling**: `RunE` returns errors up to `Execute()`, which prints `❌ <error>` and exits 1; warn but continue on non-critical errors (e.g., sound/paste)
- **Functions**: One responsibility per function; helpers at bottom
- **Flags**: cobra/pflag (`cmd.Flags()`/`cmd.PersistentFlags()`), not the stdlib `flag` package — see `cobra-cli` skill for conventions
- **Output**: Use `fmt.Println` for status, `fmt.Fprintf(os.Stderr, ...)` for errors; emoji-prefixed messages (🎤, ✅, ❌, ⚠️)
- **Concurrency**: Use goroutines for background sound playback; don't block recording
- **Subprocess**: Redirect cmd.Stderr/Stdout to user (for transparency and debugging)
- **Packages**: Clear separation - `cmd/` (entry point), `internal/` (private), `pkg/` (public/reusable)
- **Resource cleanup**: always check `os.Open`/HTTP response errors and `defer Close()` — a past leak in the voxtral health check (unclosed response body) is exactly the class of bug to avoid here.

## CLI Flags
- `--engine string` (default "whisper") - Transcription engine: "whisper" (whisper.cpp subprocess) or "voxtral" (mlx-engine HTTP server)
- `--context string` - Custom context file path (overrides global ~/.whisper-context) — whisper engine only, not yet sent to voxtral
- `--dir string` - Change working directory before recording
- `--lang string` (default "en") - Language code (en, es, fr, de, etc.)
- `--model string` (default "base") - Model size: "base" (141MB, accurate) or "tiny" (74MB, faster) — whisper engine only
- `--output string` - Save transcription to file (in addition to clipboard)
- `--no-paste` - Skip auto-paste to cursor (still copies to clipboard)
- `--no-sound` - Disable Blow.aiff and Pop.aiff audio cues
- `--verbose` (default true) - Show processing status messages (🎤, 🧠, ✅, etc.)

## CLI Subcommands
- `local-whisper engine start` / `stop` / `status` - Manage the `mlx-engine` background server (wraps `scripts/mlx-engine-server.sh`; override its path with `--script`)

## Key Functions
- `recording.Recorder.Record()` - Records with sox, 2s silence detection (3% threshold) and peak normalization (norm -3) in the same invocation, plays Blow.aiff in background
- `whisper.Client.Transcribe()` (pkg/stt/whisper) - Runs whisper-cli with model/language selection, reads transcript
- `mlx.Client.Transcribe()` (pkg/mlx) - POSTs audio (multipart) to mlx-engine's `/transcribe`, returns text — satisfies `stt.Client` alongside `whisper.Client`
- `clipboard.CopyToClipboard()` - Uses pbcopy (macOS)
- `clipboard.PasteWithAppleScript()` - Auto-pastes via osascript (requires Accessibility permissions)
- `clipboard.PlaySound()` - Async afplay (non-blocking)

## Testing
- ~37 test functions across `cmd/local-whisper`, `pkg/stt/whisper`, `pkg/mlx`, `internal/clipboard`, `internal/recording`, `internal/procutil` (fixture-driven: `testdata/bin/` fake executables + `httptest`) — `internal/audio` and `pkg/stt` (the top-level Options/Client interface) have no test files (constants/interface only, nothing to unit-test)
- Run with: `make test`
- Tests cover initialization, path handling, model validation, clipboard operations, HTTP client behavior, subprocess/signal helpers

## Signal Handling
- Catches SIGINT (Ctrl+C) and SIGTERM for graceful shutdown
- Prints "⏹️ Recording cancelled." and exits cleanly
