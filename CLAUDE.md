# CLAUDE.md - Project Instructions

Four components in this repo: the Go CLI (below), `mlx-engine/` (Python STT+TTS
server), `scripts/` (Claude Code voice hooks), `ClaudeVoiceMenuBar/` (Swift
menu bar app). See each one's own README for build/run instructions specific
to it — this file covers the Go CLI.

## Build & Run

```bash
make build              # Build binary to bin/local-whisper
make test                # Run all tests (go test -v ./...)
go vet ./...              # Check for issues
make clean                # Remove bin/
make start-engine          # Start mlx-engine (needed for -engine voxtral)
```

## Project Structure

- `cmd/local-whisper/` - CLI entry point
- `internal/recording/` - Audio recording via sox
- `internal/audio/` - Audio normalization/processing
- `internal/clipboard/` - macOS clipboard + paste via AppleScript
- `pkg/whisper/` - whisper-cli subprocess wrapper (`-engine whisper`, default)
- `pkg/voxtral/` - mlx-engine HTTP client (`-engine voxtral`)
- `mlx-engine/` - the local STT/TTS server itself (Python, `uv`-managed — see `mlx-engine/README.md`)
- `scripts/` - dependency/model setup, plus the Claude Code voice hooks (see `docs/claude-code-voice-hooks.md`)
- `ClaudeVoiceMenuBar/` - menu bar app for tuning voice settings (Swift, see its own README)

## Conventions

- **stdlib only** - No third-party Go dependencies (the Python/Swift components have their own dependency managers — `uv` and SwiftPM respectively)
- **macOS-specific** - Uses `afplay`, `pbcopy`, AppleScript, sox, whisper-cli
- **Error handling** - Return errors up; `fmt.Fprintf(os.Stderr, ...)` + `os.Exit(1)` at top level
- **Resource cleanup** - Always check `os.Open`/HTTP response errors and `defer Close()`
- **Temp files** - Written to `/tmp/voice-input/`, cleaned up on exit
- **Ports** - `mlx-engine` binds `127.0.0.1:8765` — don't reuse this port for anything else in this repo; it was previously 8000, which is common enough for other local dev servers (Django, Docker port mappings, etc.) to collide with
