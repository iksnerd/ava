# local-whisper

A lightweight Go CLI tool for local voice transcription. Records audio, processes it with sox, and transcribes using OpenAI's Whisper model—all on your machine, no cloud, no data leaves your device.

## Features

- **Local & Private**: Powered by `whisper.cpp` and `sox`. No data leaves your machine.
- **Instant Record**: Records the moment you run the command. Sound cue plays in background.
- **Smart Silence Detection**: Stops after 2 seconds of silence (3% threshold) to capture natural speech pauses.
- **Pro Audio Processing**: Normalizes audio with rate conversion for maximum accuracy.
- **Context Awareness**: Loads project vocabulary from `.whisper-context` files for better accuracy.
- **Auto-Paste**: Automatically copies to clipboard and pastes into your editor (Cmd+V).
- **Sound Feedback**: Audio cues for recording start (Blow.aiff) and stop (Pop.aiff).
- **Flexible Output**: Copy to clipboard, save to file, display in terminal, or suppress output.
- **Raycast Integration**: One-command access via Raycast (configurable hotkey).

## Quick Start

### Raycast (Recommended)

```bash
git clone https://github.com/iksnerd/local-whisper.git
cd local-whisper
make install-raycast  # Builds binary, downloads model, installs Raycast command
```

Then:
1. Open **Raycast Settings** (Cmd+,)
2. Go to **Extensions** → Scroll down to **Transcribe Local Whisper**
3. Click three dots (⋯) → **Set Hotkey** → Press hotkey (e.g., **Cmd+Shift+V**) → **Save**

Press your hotkey anytime to start transcribing!

### CLI

```bash
git clone https://github.com/iksnerd/local-whisper.git
cd local-whisper
make install-bin  # Builds binary, downloads model, installs to ~/.local/bin

# Then use directly
local-whisper
```

## Installation

### Prerequisites

```bash
brew install sox whisper-cpp go  # Go 1.25+
```

### Full Install (Automatic Model Download)

```bash
git clone https://github.com/iksnerd/local-whisper.git
cd local-whisper

# Choose one:
make install-raycast  # Install Raycast command (+ model)
make install-bin      # Install CLI to ~/.local/bin (+ model)
make setup-model      # Just download model
```

The `make install-*` commands automatically:
- ✅ Build the binary
- ✅ Download Whisper model if missing (~141MB)
- ✅ Install to the appropriate location

### Manual Build & Install

```bash
make build            # Binary → bin/local-whisper
make setup-model      # Download model to ~/.local/share/whisper-cpp/
./bin/local-whisper   # Run directly
```

### Grant Accessibility Permissions (For Auto-Paste)

For `Cmd+V` auto-paste to work:
1. Open **System Settings** → **Privacy & Security** → **Accessibility**
2. Add **Terminal**, **Raycast**, or your editor
3. Enable the toggle

## Usage

### Basic

```bash
local-whisper
```

Starts recording immediately (Blow.aiff plays), stops after 2 seconds of silence, transcribes, copies to clipboard, auto-pastes (Cmd+V), and plays Pop.aiff.

### Options

```bash
# Show help
local-whisper -help

# Use tiny model (faster, ~74MB, lower accuracy)
local-whisper -model tiny

# Transcribe Spanish
local-whisper -lang es

# French transcription with tiny model
local-whisper -model tiny -lang fr

# Silent mode (no sounds)
local-whisper -no-sound

# Don't auto-paste
local-whisper -no-paste

# Save transcription to file
local-whisper -output transcription.txt

# Use custom context file
local-whisper -context ~/my-context.txt

# Work in specific directory
local-whisper -dir /path/to/project

# Quiet mode (no status messages)
local-whisper -verbose=false
```

### Combine Flags

```bash
local-whisper -dir ~/projects/my-app -context .whisper-context -output transcript.txt -lang en -model base
```

## Context Files (God Mode)

Create a `.whisper-context` file in your project to teach Whisper your vocabulary.

### Global Context

```bash
nano ~/.whisper-context
```

Example for Go development:

```
Go, Golang, func, struct, interface, package, import, var, const, type, return, defer, go, 
select, chan, make, map, slice, range, nil, error, panic, recover, goroutine, channel, receiver
```

### Local Context

Create `.whisper-context` in your project directory for project-specific keywords:

```bash
echo "useEffect, useState, Redux, async, await" > .whisper-context
```

Local context overrides global context.

## Troubleshooting

**"No speech detected" / Recording keeps going**
- Ensure your microphone is working (System Settings → Sound)
- Speak clearly and at normal volume after the Blow.aiff sound
- Silence detection requires 2+ seconds of quiet. Don't pause mid-sentence.
- Check mic input levels (System Settings → Sound → Input)

**Sound doesn't play or auto-paste doesn't work**
- For auto-paste: Verify Accessibility permissions (System Settings → Privacy & Security → Accessibility)
- Add Terminal, Raycast, or your editor to the list
- Try without auto-paste: `local-whisper -no-paste`

**"Command not found: whisper-cli"**
- Run: `brew install whisper-cpp`

**"Model not found"**
- Run: `make setup-model` (auto-downloads)
- Or manually: `wget -O ~/.local/share/whisper-cpp/ggml-base.en.bin https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin`
- Verify: `ls -lh ~/.local/share/whisper-cpp/` (should be ~141MB)

**Slow transcription**
- First run loads the 141MB model (~5-10 seconds). Subsequent runs are much faster.
- Use tiny model for speed: `-model tiny` (74MB, faster, lower accuracy)

**Recording doesn't start immediately**
- It does—recording starts right away while Blow.aiff plays
- Speak as soon as you hear the sound

## Development

### Project Structure

```
cmd/local-whisper/     - CLI entry point (main.go)
internal/
  ├── audio/           - Audio normalization (processor.go)
  ├── clipboard/       - Clipboard & paste operations
  ├── recording/       - Audio recording (recorder.go)
pkg/whisper/           - Public Whisper wrapper (reusable library)
scripts/setup-model.sh - Model download script
Makefile               - Build automation
```

### Build & Test

```bash
make build             # Build binary to bin/local-whisper
make test              # Run all tests (13 test functions)
make clean             # Remove bin/ directory
make setup-model       # Download Whisper model
```

### Run in Development

```bash
go run ./cmd/local-whisper [flags]
```

### Code Style

- Go 1.25+, standard library only
- World-class project structure (cmd/, internal/, pkg/)
- Unit tests for all packages
- Single responsibility per function
- Descriptive naming, clear error handling

See `AGENTS.md` for detailed architecture and code guidelines.

## License

MIT
