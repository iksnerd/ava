# local-whisper

Local voice tools for your Mac: dictation, text-to-speech, and spoken Claude
Code notifications — everything runs on-device, nothing leaves your machine.

Started as a Go CLI wrapping `whisper.cpp` for dictation. It's grown into a
small local-voice stack: two interchangeable transcription engines, a
real-time call-transcription monitor, a local TTS server, Claude Code hooks
that speak session status out loud, and a menu bar app to tune it all.

## Features

- **Local & Private**: transcription and speech synthesis both run on-device — no cloud, no external data transmission.
- **Two transcription engines**: `whisper.cpp` (default, zero extra setup) or Voxtral via a local MLX server (`-engine voxtral`, higher accuracy — see [Voice Engines](#voice-engines) below).
- **Instant Recording**: starts recording immediately with audio feedback.
- **Silence Detection**: stops after 2 seconds of silence (3% threshold).
- **Context Awareness**: reads `.whisper-context` files for vocabulary hints (whisper engine).
- **Clipboard Integration**: copies to clipboard and optionally pastes via Cmd+V.
- **Raycast Integration**: available as a Raycast command.
- **Real-time call transcription**: `voice-monitor` streams a live transcript (with optional speaker diarization) from the mic or a loopback device like BlackHole, to a browser tab and a log file — see [`SETUP.md`](SETUP.md).
- **Text-to-speech**: a local Kokoro TTS server for anything that wants to speak, not just this CLI — see [`mlx-engine/`](mlx-engine/README.md).
- **Spoken Claude Code notifications**: hooks that speak when Claude finishes a turn or needs a decision — see [Claude Code voice hooks](docs/claude-code-voice-hooks.md).
- **Claude Voice menu bar app**: a one-click Dictate button (same dictation as the CLI, no terminal needed), a global Mute switch (also a system-wide keyboard-shortcut Service), plus live tuning of speed/volume/voice/message-length — see [`ClaudeVoiceMenuBar/`](ClaudeVoiceMenuBar/README.md).

## Getting Started

### Prerequisites

```bash
brew install sox whisper-cpp go
```

The default `whisper` engine works on any Mac. Voxtral (higher-accuracy
transcription, TTS, and call monitoring) needs Apple Silicon — it runs
on-device via [MLX](https://github.com/ml-explore/mlx).

### Install

```bash
git clone https://github.com/iksnerd/local-whisper.git
cd local-whisper
make install-bin       # builds the binary, downloads the model (~141MB), installs to ~/.local/bin
local-whisper
```

Or as a Raycast command instead of a standalone binary:

```bash
make install-raycast
```

Then in Raycast Settings:
1. Extensions → Add Script Directory → Select `~/raycast-scripts`
2. Reload Raycast (Cmd+Shift+R)
3. Search "Transcribe Local Whisper" and set a hotkey

### Enable auto-paste

System Settings → Privacy & Security → Accessibility → add Terminal (or
Raycast, or your editor). Without this, transcripts still land on the
clipboard, they just won't auto-paste — `local-whisper -no-paste` skips this
entirely.

That's it — say something, it lands wherever your cursor is.

### Manual build (no install)

```bash
make build             # binary to bin/local-whisper
make setup-model       # download the Whisper model
./bin/local-whisper
```

## Usage

### Basic

```bash
local-whisper
```

### Flags

```bash
local-whisper -help

local-whisper -engine voxtral      # Use the Voxtral MLX engine instead of whisper.cpp
local-whisper -model tiny          # Faster, ~74MB (whisper engine only)
local-whisper -lang es             # Language code (en, es, fr, de, etc.)
local-whisper -output file.txt     # Save to file
local-whisper -no-paste            # Skip auto-paste
local-whisper -no-sound            # Disable audio cues
local-whisper -verbose=false       # No status messages
local-whisper -context custom.txt  # Custom context file (whisper engine only — see Known gap below)
local-whisper -dir /path/to/dir    # Change working directory
```

### Combine Flags

```bash
local-whisper -dir ~/projects/app -lang en -model base -output transcript.txt
```

## Voice Engines

| | `whisper` (default) | `voxtral` |
|---|---|---|
| Setup | `brew install whisper-cpp`, nothing else | Needs `mlx-engine/` running — see below |
| Where it runs | Subprocess per transcription | Persistent local server (`127.0.0.1:8765`), started on demand |
| Cold start | ~2-3s every time | ~2-3s first request, then warm |
| Accuracy | ~10% WER (Whisper Large-v3-class) | ~4% WER, better with technical vocabulary |
| `.whisper-context` vocabulary hints | Yes | Not yet — see Known gap |

To use Voxtral: start the server once (`bash scripts/voxtral-server.sh start`,
or just run `local-whisper -engine voxtral` — it starts automatically),
then `local-whisper -engine voxtral`. Full detail on the server, its models,
and its HTTP API: [`mlx-engine/README.md`](mlx-engine/README.md).

**Known gap**: `.whisper-context` vocabulary hints work under `-engine
whisper` but aren't sent to the Voxtral server yet — see
[`voxtral-migration.md`](voxtral-migration.md#known-limitation).

## Context Files

Create a `.whisper-context` file to provide vocabulary hints for better transcription (whisper engine only — see Known gap above).

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
- Use tiny model for speed: `-model tiny` (whisper engine)

**"voxtral server is not running or model failed to load"**
```bash
bash scripts/voxtral-server.sh start   # or: status / stop
```
Check `/tmp/voxtral-server.log` if it doesn't come up. Full troubleshooting: [`mlx-engine/README.md`](mlx-engine/README.md).

## Documentation

- [`mlx-engine/README.md`](mlx-engine/README.md) — the local STT/TTS server: endpoints, models, running it standalone.
- [`SETUP.md`](SETUP.md) — `voice-monitor` (real-time call transcription): BlackHole loopback setup, engine/diarization options, known gaps.
- [`docs/claude-code-voice-hooks.md`](docs/claude-code-voice-hooks.md) — spoken Claude Code notifications: setup, settings, how message length/summarization work.
- [`ClaudeVoiceMenuBar/README.md`](ClaudeVoiceMenuBar/README.md) — the menu bar app for tuning the above live.
- [`voxtral-migration.md`](voxtral-migration.md) — design record for why/how the Voxtral engine was added.
- [`AGENTS.md`](AGENTS.md) — architecture and code style, for anyone (human or agent) working on this repo.

## Development

### Structure

```
cmd/local-whisper/     - CLI entry point
cmd/voice-monitor/     - realtime call-transcript monitor (see SETUP.md)
internal/
  ├── audio/           - Audio normalization
  ├── clipboard/       - Clipboard & paste operations
  ├── recording/       - Audio recording
  └── procutil/        - shared subprocess/signal helpers
pkg/whisper/            - whisper.cpp subprocess wrapper (-engine whisper)
pkg/mlxengine/          - mlx-engine HTTP client (-engine voxtral)
pkg/voxtral/            - voxtral/ Python primitives subprocess wrapper (cmd/voice-monitor)
mlx-engine/             - the local STT/TTS server itself (Python, uv-managed)
voxtral/                - Voxtral MLX primitives for voice-monitor (Python, uv-managed)
scripts/                - setup, model download, and voice-hook scripts (see docs/claude-code-voice-hooks.md)
scripts/voice_hooks/    - text processing for the voice hooks: markdown stripping, sentence-aware
                          truncation, Ollama summarization (Python, uv-managed)
ClaudeVoiceMenuBar/     - menu bar app for tuning voice settings (Swift)
Makefile                - build automation
```

### Build & Test

```bash
make build             # Build binary
make test              # Run tests
make vet                # go vet the Go code
make fmt                # Format Go (gofmt) and Python (ruff format), in place
make fmt-check          # Same, check-only (CI-safe)
make lint               # vet + fmt-check + ruff check (mlx-engine/, voxtral/, scripts/voice_hooks/)
make clean             # Remove bin/
go run ./cmd/local-whisper [flags]  # Run without building
```

Run `make lint` before committing.

### Code Guidelines

See `AGENTS.md` for detailed architecture and code style.

## License

MIT
