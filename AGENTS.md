# local-whisper Agent Guide

## Build Commands
- `make build` - Compile binary (`go build -o local-whisper`)
- `make install-raycast` - Build and install Raycast command script
- `make install-bin` - Build and install to ~/.local/bin
- `make clean` - Remove built binary
- `go run main.go [flags]` - Run directly without building
- No tests or linting currently configured

## Architecture
Single-file CLI tool (`main.go`) for local voice transcription. Wraps external `whisper-cli` command-line tool and `sox` for audio recording/processing.

**External dependencies** (not in go.mod):
- `whisper-cli` - OpenAI Whisper C++ implementation
- `sox` - Audio recording with silence detection
- `afplay` - Sound playback (macOS, async)
- `osascript` - AppleScript for auto-paste (macOS)

**Model location**: `~/.local/share/whisper-cpp/ggml-base.en.bin` (141MB, user-provided)
**Alternate model**: `ggml-tiny.en.bin` (74MB, faster but less accurate)

**Temp directory**: `/tmp/voice-input/` (raw/processed WAV files, transcript)

**Recording behavior**:
- Starts immediately while Blow.aiff plays in background
- Stops after 2 seconds of silence at 3% threshold
- Normalizes audio before transcription

## Code Style
- **Imports**: Standard library only (no external Go dependencies)
- **Naming**: CamelCase for functions; descriptive names (e.g., `transcribeAudio`, `recordAudio`, `pasteWithAppleScript`)
- **Error handling**: Check errors explicitly, exit with code 1 on failure; warn but continue on non-critical errors (e.g., sound/paste)
- **Functions**: One responsibility per function; helpers (playSound, copyToClipboard, etc.) at bottom
- **Flags**: Use standard `flag` package for CLI arguments
- **Output**: Use `fmt.Println` for status, `fmt.Fprintf(os.Stderr, ...)` for errors; emoji-prefixed messages (🎤, ✅, ❌, ⚠️)
- **Concurrency**: Use goroutines for background sound playback; don't block recording
- **Subprocess**: Redirect cmd.Stderr/Stdout to user (for transparency and debugging)

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
- `recordAudio(path, playSound)` - Records with sox, 2s silence detection (3% threshold), plays Blow.aiff in background
- `processAudio(in, out)` - Normalizes audio with rate/channel conversion
- `transcribeAudio(path, output, prompt, model, lang)` - Runs whisper-cli with model/language selection, reads transcript
- `copyToClipboard(text)` - Uses pbcopy (macOS)
- `pasteWithAppleScript()` - Auto-pastes via osascript (requires Accessibility permissions)
- `playSound(path, enabled)` - Async afplay (non-blocking)

## Signal Handling
- Catches SIGINT (Ctrl+C) and SIGTERM for graceful shutdown
- Prints "⏹️ Recording cancelled." and exits cleanly
