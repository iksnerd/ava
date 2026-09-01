# Local Whisper Project Instructions

## Build & Run

```bash
make build        # Build binary to bin/local-whisper
make test         # Run all tests (go test -v ./...)
go vet ./...      # Check for issues
make clean        # Remove bin/
```

## Project Structure

- `cmd/local-whisper/` - CLI entry point
- `internal/recording/` - Audio recording via sox
- `internal/audio/` - Audio normalization/processing
- `internal/clipboard/` - macOS clipboard + paste via AppleScript
- `pkg/stt/whisper/` - whisper-cli wrapper
- `scripts/` - Setup scripts for dependencies and model download

## Conventions

- **stdlib only** - No third-party Go dependencies
- **macOS-specific** - Uses `afplay`, `pbcopy`, AppleScript, sox, whisper-cli
- **Error handling** - Return errors up; `fmt.Fprintf(os.Stderr, ...)` + `os.Exit(1)` at top level
- **Resource cleanup** - Always check `os.Open` errors and `defer Close()`
- **Temp files** - Written to `/tmp/voice-input/`, cleaned up on exit
