# local-whisper

A Go CLI tool for local voice transcription using OpenAI's Whisper model. Records audio, processes it, and transcribes entirely on your machine—no cloud, no data leaves your device.

## Features

- **Local & Private**: Uses `whisper.cpp` and `sox`. No external data transmission.
- **Instant Recording**: Starts recording immediately with audio feedback.
- **Silence Detection**: Stops after 2 seconds of silence (3% threshold).
- **Audio Processing**: Normalizes audio with rate conversion.
- **Context Awareness**: Reads `.whisper-context` files for vocabulary hints.
- **Clipboard Integration**: Copies to clipboard and optionally pastes via Cmd+V.
- **Audio Cues**: Sound feedback for recording start and stop.
- **Flexible Output**: Copy to clipboard, save to file, or suppress output.
- **Raycast Integration**: Available as a Raycast command.

## Quick Start

### Raycast

```bash
git clone https://github.com/iksnerd/local-whisper.git
cd local-whisper
make install-raycast
```

Then in Raycast Settings:
1. Extensions → Add Script Directory → Select `/Users/[username]/raycast-scripts`
2. Reload Raycast (Cmd+Shift+R)
3. Search "Transcribe Local Whisper" and set your hotkey

### CLI

```bash
git clone https://github.com/iksnerd/local-whisper.git
cd local-whisper
make install-bin
local-whisper
```

## Installation

### Prerequisites

```bash
brew install sox whisper-cpp go
```

### Install

```bash
git clone https://github.com/iksnerd/local-whisper.git
cd local-whisper
make install-raycast   # Raycast command
# or
make install-bin       # CLI to ~/.local/bin
```

The install commands build the binary, download the model (~141MB), and set everything up.

### Manual Setup

```bash
make build             # Binary to bin/local-whisper
make setup-model       # Download model
./bin/local-whisper
```

### Accessibility (For Auto-Paste)

System Settings → Privacy & Security → Accessibility → Add Terminal/Raycast/your editor.

## Usage

### Basic

```bash
local-whisper
```

### Flags

```bash
local-whisper -help

local-whisper -model tiny          # Faster, ~74MB (tiny vs base)
local-whisper -lang es             # Language code (en, es, fr, de, etc.)
local-whisper -output file.txt     # Save to file
local-whisper -no-paste            # Skip auto-paste
local-whisper -no-sound            # Disable audio cues
local-whisper -verbose=false       # No status messages
local-whisper -context custom.txt  # Custom context file
local-whisper -dir /path/to/dir    # Change working directory
```

### Combine Flags

```bash
local-whisper -dir ~/projects/app -lang en -model base -output transcript.txt
```

## Context Files

Create a `.whisper-context` file to provide vocabulary hints for better transcription.

### Global Context

```bash
nano ~/.whisper-context
```

Example for Go:
```
Go, Golang, func, struct, interface, package, import, var, const, defer, goroutine
```

### Local Context

Create `.whisper-context` in your project directory. Local context takes precedence.

```bash
echo "useEffect, useState, Redux, async, await" > .whisper-context
```

## Troubleshooting

**Recording keeps going / No speech detected**
- Verify microphone (System Settings → Sound)
- Speak clearly after the audio cue
- Silence detection requires 2+ seconds of quiet

**Sound/auto-paste not working**
- Check Accessibility permissions (System Settings → Privacy & Security → Accessibility)
- Try without auto-paste: `local-whisper -no-paste`

**"Command not found: whisper-cli"**
```bash
brew install whisper-cpp
```

**"Model not found"**
```bash
make setup-model
```

**Slow transcription**
- First run loads the model (~5-10 seconds). Subsequent runs are faster.
- Use tiny model for speed: `-model tiny`

## Development

### Structure

```
cmd/local-whisper/     - CLI entry point
internal/
  ├── audio/           - Audio normalization
  ├── clipboard/       - Clipboard & paste operations
  ├── recording/       - Audio recording
pkg/whisper/           - Public Whisper wrapper
scripts/setup-model.sh - Model download
Makefile               - Build automation
```

### Build & Test

```bash
make build             # Build binary
make test              # Run tests (13 test functions)
make clean             # Remove bin/
go run ./cmd/local-whisper [flags]  # Run without building
```

### Code Guidelines

See `AGENTS.md` for detailed architecture and code style.

## License

MIT
