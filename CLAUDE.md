# CLAUDE.md - Project Instructions

## Build & Run

```bash
make build              # Build binary to bin/local-whisper
make build-voice-monitor # Build the realtime call-transcript monitor (see SETUP.md)
make test               # Run all tests (go test -v ./...)
go vet ./...            # Check for issues
make clean              # Remove bin/
```

## Project Structure

- `cmd/local-whisper/` - CLI entry point
- `cmd/voice-monitor/` - realtime transcript monitor: serves a live transcript over SSE at localhost and logs it to a file; input can be the mic or a loopback device (e.g. BlackHole) for capturing call audio
- `internal/recording/` - Audio recording via sox
- `internal/audio/` - Audio normalization/processing
- `internal/clipboard/` - macOS clipboard + paste via AppleScript
- `pkg/whisper/` - whisper-cli wrapper
- `pkg/voxtral/` - Go wrapper around the Voxtral MLX primitives (see `voxtral/`)
- `voxtral/` - Python/MLX primitives (uv project): Voxtral STT (Mini 3B, Mini 4B Realtime), a Whisper fallback engine (multilingual, for languages Voxtral doesn't cover), Sortformer speaker diarization, and Voxtral TTS (4B TTS); see `make setup-voxtral` and `SETUP.md`
- `scripts/` - Setup scripts for dependencies and model download

## Conventions

- **stdlib only** - No third-party Go dependencies. `voxtral/` is external Python/MLX tooling shelled out to via `os/exec`, same as whisper-cli/sox - not a Go dependency.
- **macOS-specific** - Uses `afplay`, `pbcopy`, AppleScript, sox, whisper-cli, MLX (Apple Silicon only)
- **Error handling** - Return errors up; `fmt.Fprintf(os.Stderr, ...)` + `os.Exit(1)` at top level
- **Resource cleanup** - Always check `os.Open` errors and `defer Close()`
- **Temp files** - Written to `/tmp/voice-input/`, cleaned up on exit
