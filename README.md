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

## Quick Start with Raycast

1. Build and install the Raycast command:
   ```bash
   make install-raycast
   ```

2. Open **Raycast Settings** (Cmd+,)

3. Go to **Extensions** → Scroll down to **Transcribe Local Whisper**

4. Click the three dots (⋯) and select **Set Hotkey**

5. Press your desired hotkey (e.g., **Cmd+Shift+V**)

6. Click **Save**

Now press your hotkey anytime to start transcribing!

## Installation

### 1. Install Dependencies

```bash
# Install recording and transcription tools
brew install sox whisper-cpp

# Create directory for models
mkdir -p ~/.local/share/whisper-cpp

# Download the Base Model (best balance of speed/accuracy)
wget -O ~/.local/share/whisper-cpp/ggml-base.en.bin \
  https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin
```

### 2. Build local-whisper

```bash
cd /Users/user/GolandProjects/local-whisper
go build -o local-whisper
```

### 3. Install Binary (Optional)

```bash
# Move to /usr/local/bin to run from anywhere
sudo mv local-whisper /usr/local/bin/

# Or add to your PATH
export PATH="$PATH:$(pwd)"
```

### 4. Grant Permissions (Critical)

For auto-paste to work:
1. Open **System Settings** → **Privacy & Security** → **Accessibility**
2. Add **Terminal** (or your editor) to the list
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
- Run the download command in Installation step 1
- Verify: `ls -lh ~/.local/share/whisper-cpp/`
- Model should be ~141MB

**Slow transcription**
- First run loads the 141MB model (~5-10 seconds). Subsequent runs are much faster.
- Use smaller model: `wget -O ~/.local/share/whisper-cpp/ggml-tiny.en.bin https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.en.bin`

**Recording doesn't start immediately**
- It does—recording starts right away while Blow.aiff plays
- Speak as soon as you hear the sound

## Development

### Run without building

```bash
go run main.go [flags]
```

### Rebuild and reinstall

```bash
# Just rebuild
make build

# Rebuild and install to ~/.local/bin
make install-bin

# Rebuild and install Raycast command
make install-raycast
```

### Clean up

```bash
make clean
```

See `AGENTS.md` for architecture and code style details.

## License

MIT
