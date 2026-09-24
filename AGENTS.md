# Ava Agent Guide

This repo is five components: the `ava` Go CLI (dictation, speech,
transcription, accessibility narration, an MCP server, and `ava monitor` for
realtime call transcripts in `internal/monitor`), the `mlx-engine` Python server (local
Kokoro TTS, used by the CLI, the hooks and the menu bar app), `voxtral/` (Python/MLX
primitives used by `ava monitor`), `scripts/` (Claude Code voice hooks), and
`AvaMenuBar` (a Swift menu bar app for tuning voice settings). This
guide covers the Go CLI (`ava`) specifically; see each other
component's own docs — `docs/monitor.md` (`ava monitor`), `mlx-engine/README.md`,
`docs/claude-code-voice-hooks.md`, `AvaMenuBar/README.md`,
`docs/mcp.md`.

## Build Commands
- `make build` - Compile binary to `bin/ava`
- `make test` - Run all Go tests, the `scripts/voice_hooks/` pytest suite, and the `mlx-engine` and `voxtral` test suites (`make test-mlx-engine`, `make test-voxtral`)
- `make lint` - `go vet`, format check, `check-paths`, `check-names`, `check-protocol`, `check-enginedist`, `check-docs`, then `ruff check` on the three Python projects
- `make check-swift-config` - Check VoiceSettings.swift resolves the voice config the same way bash and Go do (needs swift; not part of `make test`)
- `make check-paths` - Fail if any tracked file hardcodes a home directory (part of `make lint`)
- `make setup-model` - Download Whisper model to ~/.local/share/whisper-cpp/ (pinned revision, sha256-verified; `ava setup-model` does the same without a checkout)
- `make install-raycast` - Build, download model, install Raycast command
- `make install-bin` - Build, download model, install to ~/.local/bin
- `make uninstall` - Remove what `install-bin`, `install-raycast` and `ava setup` installed; see `docs/uninstall.md` for what it leaves
- `make start-engine` / `stop-engine` / `status-engine` - Manage the `mlx-engine` background server (Kokoro TTS); thin wrappers around `ava engine start` / `stop` / `status`
- `make clean` - Remove bin/ directory
- `go run ./cmd/ava [flags]` - Run directly without building

## Architecture
Multi-package CLI tool for local voice, in both directions. Transcription is
`whisper.cpp` via subprocess, the only engine and zero extra setup, behind the
`pkg/stt.Client` interface. Synthesis is Kokoro, reached over HTTP through
`pkg/mlx/` to the local `mlx-engine/` server.

`mlx-engine/` used to serve STT as well, behind `--engine voxtral`. That was
removed after measuring it: whisper.cpp took 1.26s on a 20s sample against
Voxtral's 17s warm and 127s cold, for a near-identical transcript. Voxtral
remains in `voxtral/` for `internal/monitor`, whose job is streaming, where
`pkg/stt/whisper` transcribes a complete file per subprocess.

**Project Structure:**
```
cmd/ava/main.go       - func main() { Execute() }
cmd/ava/root.go       - cobra root command: flags, RunE (record → transcribe → output)
cmd/ava/engine.go     - `engine` subcommand group (start/stop/status), wraps
                                  scripts/mlx-engine-server.sh
cmd/ava/model.go      - --model and --beam-size validation, model filename selection
cmd/ava/context.go    - .whisper-context loading
cmd/ava/dependencies.go - external dependency checks (sox, whisper-cli)
cmd/ava/speak.go      - `speak` (Kokoro or the `say` fallback; reads stdin with no args)
cmd/ava/stop.go       - `stop` (cancels speech from any source, incl. the hooks)
cmd/ava/voices.go     - `voices`, plus formatVoices() shared with the MCP list_voices tool
cmd/ava/transcribe.go - `transcribe <file>` (an existing WAV, vs. the root command recording one)
cmd/ava/a11y.go       - `a11y` (accessibility tree -> screen-reader announcements)
cmd/ava/mcp.go        - `mcp`: stdio MCP server exposing the five above to any MCP client.
                                  Nothing in this command may write to stdout — that's the JSON-RPC
                                  channel. See docs/mcp.md
cmd/ava/setup.go      - `setup`: sox + whisper-cli via Homebrew, the model, and the Kokoro
                                  engine bundle (from internal/enginedist); --skip-engine
cmd/ava/setup_model.go - `setup-model`: pinned-revision, sha256-verified model download
internal/recording/recorder.go  - Audio recording, silence detection, and peak
                                  normalization (norm -3) in one sox invocation
internal/audio/audio.go         - shared sox format constants (SampleRateHz, Channels)
internal/clipboard/clipboard.go - Clipboard & auto-paste operations
internal/procutil/              - shared subprocess/signal helpers
internal/buildinfo/             - which build is running (`--version`, the MCP handshake)
internal/enginedist/            - the mlx-engine bundle embedded in the binary, so `setup` can
                                  install Kokoro without a checkout; `make check-enginedist`
                                  fails if it drifts from mlx-engine/
internal/protocol/              - marker-file protocol constants, generated from protocol.json
                                  into Go, bash and Swift; `make check-protocol`
internal/ttsproto/              - the Go-side names for those constants, plus ActivityDir(),
                                  which honours the test override; every Go speak/stop path
                                  reads them from here
internal/testutil/              - helpers shared by the test suites
internal/voiceconfig/           - the live Ava settings (mute, speed, volume, voice,
                                  say rate). Hand-ported from scripts/lib.sh's config_get; drift
                                  tests pin the defaults to scripts/voice-defaults.json and the
                                  voice list to VoiceSettings.swift. contract_test.go runs bash
                                  and Go against testdata/voice-config-cases.json and fails if the
                                  two readers of this config disagree; `make check-swift-config`
                                  is the Swift arm (needs swiftc, so not in `make test`)
internal/speaker/               - synthesis + playback; scripts/speak.sh hands off to it
                                  (mute gate, activity marker, shared flock, `say` fallback)
internal/ttscontrol/            - cancels speech in flight (Go port of stop-speaking.sh)
internal/a11y/                  - accessibility tree -> screen-reader announcements + findings.
                                  Pure functions; rendering must be identical run to run
pkg/stt/stt.go                  - Options/Client shapes of the one-shot STT engine,
                                  plus WriteOutputIfRequested()
pkg/stt/whisper/whisper.go      - whisper-cli subprocess wrapper; ava's only
                                  transcription engine
pkg/mlx/mlx.go                  - mlx-engine HTTP client: Speak and Healthy. Lives at the
                                  top level, not under pkg/stt, because it is not STT
mlx-engine/                     - the local Kokoro TTS server (separate Python/uv project)
scripts/setup-model.sh          - Auto-download whisper model script
scripts/mlx-engine-server.sh    - Start/stop/status for mlx-engine, wrapped by `ava engine`
```

Not covered here: `pkg/stt/realtime` (a *different*, independent client —
wraps `voxtral/realtime.py` via `os/exec`, used only by `internal/monitor`)
and `voxtral/` itself. See `docs/monitor.md`. There is no `pkg/tts`:
synthesis is `pkg/mlx.Client.Speak` (same server, same HTTP-client
scaffolding), wrapped by `internal/speaker` (see `pkg/stt`'s own package doc
comment for why).

**External dependencies** (not in go.mod):
- `whisper-cli` - OpenAI Whisper C++ implementation; the transcription engine
- `sox` - Audio recording with silence detection
- `afplay` - Sound playback (macOS, async)
- `osascript` - AppleScript for auto-paste (macOS)
- `uv` - runs `mlx-engine`, the Kokoro TTS server (start it with `ava engine start`, which shells out to `scripts/mlx-engine-server.sh`)

**Model location**: `~/.local/share/whisper-cpp/ggml-base.en.bin` (141MB, downloaded by `make setup-model` or `ava setup-model`)
**Alternate model**: `ggml-tiny.en.bin` (74MB, faster but less accurate)

**Voxtral**: used only by `ava monitor` (`internal/monitor`), through `voxtral/` and `pkg/stt/realtime`, never by dictation or `ava transcribe`. See `docs/monitor.md`.

**Temp directory**: `/tmp/voice-input/` (raw/processed WAV files, transcript)

**Recording behavior**:
- Starts immediately while Blow.aiff plays in background
- Stops after 2 seconds of silence at 3% threshold
- Normalizes audio before transcription

## Code Style
- **Imports**: two external Go dependencies — `github.com/spf13/cobra` (and its `pflag` dependency) for the CLI command tree, and `github.com/modelcontextprotocol/go-sdk` used only by `cmd/ava/mcp.go`; everything else is stdlib. Don't add a third without the same kind of reason
- **Naming**: CamelCase for functions; descriptive names (e.g., `recordAudio`, `pasteWithAppleScript`)
- **Error handling**: `RunE` returns errors up to `Execute()`, which prints `❌ <error>` and exits 1; warn but continue on non-critical errors (e.g., sound/paste)
- **Functions**: One responsibility per function; helpers at bottom
- **Flags**: cobra/pflag (`cmd.Flags()`/`cmd.PersistentFlags()`), not the stdlib `flag` package
- **Output**: Use `fmt.Println` for status, `fmt.Fprintf(os.Stderr, ...)` for errors; emoji-prefixed messages (🎤, ✅, ❌, ⚠️)
- **Concurrency**: Use goroutines for background sound playback; don't block recording
- **Subprocess**: Redirect cmd.Stderr/Stdout to user (for transparency and debugging)
- **Packages**: Clear separation - `cmd/` (entry point), `internal/` (private), `pkg/` (public/reusable)
- **Resource cleanup**: always check `os.Open`/HTTP response errors and `defer Close()` — a past leak in the voxtral health check (unclosed response body) is exactly the class of bug to avoid here.

## CLI Flags
- `--context string` - Custom context file path (overrides global ~/.whisper-context)
- `--dir string` - Change working directory before recording
- `--lang string` (default "en") - Language code (en, es, fr, de, etc.)
- `--model string` (default "base") - Model size: "base" (141MB, accurate) or "tiny" (74MB, faster)
- `--beam-size int` (default 0) - whisper.cpp beam width; 0 keeps whisper's default of 5, lower is faster and less accurate. Also on `transcribe`, and `beam_size` on the MCP `transcribe` tool
- `--output string` - Save transcription to file (in addition to clipboard)
- `--no-paste` - Skip auto-paste to cursor (still copies to clipboard)
- `--no-sound` - Disable Blow.aiff and Pop.aiff audio cues
- `--verbose` (default true) - Show processing status messages (🎤, 🧠, ✅, etc.)
- `--quiet` - Inverse of `--verbose`, for symmetry with `a11y --quiet`

## CLI Subcommands
- `speak [text...]` - Speak via Kokoro, or `say` if the server is down; reads stdin with no args. `--voice`, `--speed`, `--async`, `--server-url`
- `stop` - Cancel speech in flight from any source, including the hooks
- `voices` - List the Kokoro voice ids `--voice` accepts
- `transcribe <audio.wav>` - Transcribe an existing file. `--lang`, `--model`, `--beam-size`, `--output`
- `a11y [snapshot-file]` - Narrate a chrome-devtools `take_snapshot` tree. `--mode`, `--quiet`, `--voice`, `--speed`, `--server-url`
- `mcp` - Serve the above over stdio MCP; see `docs/mcp.md`
- `setup` - Install sox, whisper-cli, the base model and the Kokoro engine; `--skip-engine` for dictation only
- `setup-model` - Download a whisper.cpp model (`--model base|tiny`), pinned and sha256-verified
- `engine start` / `stop` / `status` - Manage the `mlx-engine` background server (wraps `scripts/mlx-engine-server.sh`, the repo's or the one `setup` installed; override with `--script`). `stop` also stops hooks auto-starting it; `stop --keep-autostart` leaves that armed

## Key Functions
- `recording.Recorder.Record()` - Records with sox, 2s silence detection (3% threshold) and peak normalization (norm -3) in the same invocation, plays Blow.aiff in background
- `whisper.Client.Transcribe()` (pkg/stt/whisper) - Runs whisper-cli with model/language selection, reads transcript
- `mlx.Client.Speak()` (pkg/mlx) - POSTs text to mlx-engine's `/speak`, returns the audio; `internal/speaker` wraps it with the mute gate, activity marker and playback lock
- `clipboard.CopyToClipboard()` - Uses pbcopy (macOS)
- `clipboard.PasteWithAppleScript()` - Auto-pastes via osascript (requires Accessibility permissions)
- `clipboard.PlaySound()` - Async afplay (non-blocking)

## Testing
- Go tests in `cmd/ava`, `internal/monitor`, `pkg/stt/whisper`, `pkg/stt/realtime`, `pkg/mlx`, and `internal/{a11y,buildinfo,clipboard,enginedist,procutil,protocol,recording,speaker,ttscontrol,voiceconfig}` (fixture-driven: `testdata/bin/` fake executables, `httptest`, and the MCP SDK's in-memory transport for the `mcp` command) — `internal/audio`, `internal/ttsproto`, `internal/testutil` and `pkg/stt` have no test files (constants, helpers and interface only)
- Run with: `make test`
- Tests cover initialization, path handling, model validation, clipboard operations, HTTP client behavior, subprocess/signal helpers

## Signal Handling
- Catches SIGINT (Ctrl+C) and SIGTERM for graceful shutdown
- Prints "⏹️ Recording cancelled." and exits cleanly
