# local-whisper Agent Guide

## Build Commands
- `make build` - Compile binary to `bin/local-whisper`
- `make test` - Run all unit tests (13 test functions across 4 packages)
- `make setup-model` - Download Whisper model to ~/.local/share/whisper-cpp/
- `make install-raycast` - Build, download model, install Raycast command
- `make install-bin` - Build, download model, install to ~/.local/bin
- `make clean` - Remove bin/ directory
- `go run ./cmd/local-whisper [flags]` - Run directly without building

## Architecture
Multi-package CLI tool for local voice transcription. Wraps external `whisper-cli` command-line tool and `sox` for audio recording/processing.

**Project Structure:**
```
cmd/local-whisper/main.go      - CLI entry point
internal/recording/recorder.go - Audio recording with silence detection
internal/audio/processor.go    - Audio normalization
internal/clipboard/clipboard.go - Clipboard & auto-paste operations
pkg/whisper/whisper.go         - Public Whisper client wrapper
scripts/setup-model.sh         - Auto-download model script
```

**External dependencies** (not in go.mod):
- `whisper-cli` - OpenAI Whisper C++ implementation
- `sox` - Audio recording with silence detection
- `afplay` - Sound playback (macOS, async)
- `osascript` - AppleScript for auto-paste (macOS)

**Model location**: `~/.local/share/whisper-cpp/ggml-base.en.bin` (141MB, auto-downloaded by `make setup-model`)
**Alternate model**: `ggml-tiny.en.bin` (74MB, faster but less accurate)

**Temp directory**: `/tmp/voice-input/` (raw/processed WAV files, transcript)

**Recording behavior**:
- Starts immediately while Blow.aiff plays in background
- Stops after 2 seconds of silence at 3% threshold
- Normalizes audio before transcription

## Code Style
- **Imports**: Standard library only (no external Go dependencies)
- **Naming**: CamelCase for functions; descriptive names (e.g., `recordAudio`, `pasteWithAppleScript`)
- **Error handling**: Check errors explicitly, exit with code 1 on failure; warn but continue on non-critical errors (e.g., sound/paste)
- **Functions**: One responsibility per function; helpers at bottom
- **Flags**: Use standard `flag` package for CLI arguments
- **Output**: Use `fmt.Println` for status, `fmt.Fprintf(os.Stderr, ...)` for errors; emoji-prefixed messages (🎤, ✅, ❌, ⚠️)
- **Concurrency**: Use goroutines for background sound playback; don't block recording
- **Subprocess**: Redirect cmd.Stderr/Stdout to user (for transparency and debugging)
- **Packages**: Clear separation - `cmd/` (entry point), `internal/` (private), `pkg/` (public/reusable)

## CLI Flags
- `-context string` - Custom context file path (overrides global ~/.whisper-context)
- `-dir string` - Change working directory before recording
- `-lang string` (default "en") - Language code (en, es, fr, de, etc.)
- `-model string` (default "base") - Model size: "base" (141MB, accurate) or "tiny" (74MB, faster)
- `-output string` - Save transcription to file (in addition to clipboard)
- `-no-paste` - Skip auto-paste to cursor (still copies to clipboard)
- `-no-sound` - Disable Blow.aiff and Pop.aiff audio cues
- `-verbose` (default true) - Show processing status messages (🎤, 🧠, ✅, etc.)

## Key Functions
- `recording.Recorder.Record()` - Records with sox, 2s silence detection (3% threshold), plays Blow.aiff in background
- `audio.Processor.Normalize()` - Normalizes audio with rate/channel conversion
- `whisper.Client.Transcribe()` - Runs whisper-cli with model/language selection, reads transcript
- `clipboard.CopyToClipboard()` - Uses pbcopy (macOS)
- `clipboard.PasteWithAppleScript()` - Auto-pastes via osascript (requires Accessibility permissions)
- `clipboard.PlaySound()` - Async afplay (non-blocking)

## Testing
- 13 test functions across 4 packages (`pkg/whisper`, `internal/audio`, `internal/clipboard`, `internal/recording`)
- Run with: `make test`
- Tests cover initialization, path handling, model validation, clipboard operations

## Signal Handling
- Catches SIGINT (Ctrl+C) and SIGTERM for graceful shutdown
- Prints "⏹️ Recording cancelled." and exits cleanly
