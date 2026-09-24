# Getting started on a new Mac

A checklist for setting Ava up on a Mac from nothing, written so someone other than the owner can
follow it. Every step ends with a check; don't move on until it passes.

← [Back to the README](../README.md)

**Which path?** Steps 0 to 5 install `ava` from a release: dictation, natural-voice speech,
transcribing files, and the MCP server for Claude Code. The menu bar app, the Claude Code voice
hooks and live call transcripts need the source: [Full install from source](#full-install-from-source).

## 0. Check the machine

```bash
uname -m                 # arm64 = Apple Silicon: everything works
sw_vers -productVersion  # 13 or later if you want the menu bar app
```

Release binaries are Apple Silicon only. On an Intel Mac (`x86_64`), see the Intel note under
[Full install from source](#full-install-from-source); only dictation works there.

## 1. Homebrew

`ava setup` installs its tools through [Homebrew](https://brew.sh). If `brew --version` does not
run, install it from brew.sh: paste its one-line command, then run the `echo … >> ~/.zprofile`
lines it prints at the end, and open a new terminal window.

Check: `brew --version` prints a version.

## 2. Install ava

```bash
curl -fsSL https://raw.githubusercontent.com/iksnerd/ava/main/scripts/install.sh | bash
```

It downloads the latest release, checks it against the published checksum, and installs `ava` to
`~/.local/bin`. If it prints a line starting `echo 'export PATH=`, run that
line, then open a new terminal window.

Check: `ava --version` prints a version.

A specific version instead of the latest: add `-s v0.7.0` after `bash`.

**Downloading from the Releases page instead.** The binaries are unsigned (there is no paid Apple
Developer account behind them), so macOS quarantines anything a browser downloads, and it kills a
quarantined unsigned binary without a message. After extracting the `.tar.gz`, in that folder:

```bash
shasum -a 256 -c checksums.txt --ignore-missing
xattr -d com.apple.quarantine ava
mkdir -p ~/.local/bin && mv ava ~/.local/bin/
```

## 3. Install what ava needs

```bash
ava setup
```

This installs `sox`, `whisper-cli` and `uv` through Homebrew, the 141 MB speech model, and the Kokoro
voice engine with its model and every voice (about 1.5 GB, a few minutes the first time). After
it, speech needs no network. Every step is skipped when it is
already done, so if anything fails, fix it and run `ava setup` again.

Check: it ends with `✅ Setup complete.`

## 4. Speech

```bash
ava speak "hello"
```

The first time, the engine starts, which takes a few seconds; `ava setup` already downloaded its
model. You should hear a natural voice. If `ava speak` prints a warning about the macOS `say`
voice instead, run `ava engine start`, wait for "ready", and try again.

Check: a natural-sounding voice says "hello".

## 5. Dictation and the two permissions

Dictate once from the terminal, without pasting:

```bash
ava --no-paste       # say a sentence, then stay quiet for 2 seconds
```

- macOS asks for **Microphone** access for your terminal app. Allow it. If it never asked and
  nothing was transcribed, add the terminal under System Settings → Privacy & Security →
  Microphone.
- Now run plain `ava`. Pasting needs **Accessibility**: allow it for the terminal under System
  Settings → Privacy & Security → Accessibility.

Check: the transcript lands at the cursor. On this first run the cursor is in the terminal itself,
so the words appear at your own shell prompt. That is correct: normally you switch to the app you
want to type into first, then run `ava`.

That is the release install done. Optional:

- **Claude Code can use Ava's voice and transcription:**
  `claude mcp add -s user ava -- "$HOME/.local/bin/ava" mcp`, then restart Claude Code. See
  [mcp.md](mcp.md).
- **Every command and flag:** `ava --help`, or [cli.md](cli.md).

## Full install from source

Needed for the menu bar app, the Claude Code voice hooks and live call transcripts. Do step 0 and
step 1 above first.

```bash
xcode-select --install          # git and Swift; skip if already installed
brew install go sox whisper-cpp
```

Pick the folder now and keep it: the menu bar app and the voice hooks record this path, so moving
it later means rebuilding the app and editing the hooks.

```bash
mkdir -p ~/src && cd ~/src
git clone https://github.com/iksnerd/ava.git
cd ava
make setup          # uv, the Kokoro Python environment (~1.2 GB) and the speech model
make install-bin    # builds ava and installs it to ~/.local/bin
```

Add these two lines to `~/.zshrc`, then open a new terminal window:

```bash
export PATH="$HOME/.local/bin:$PATH"
export MLX_ENGINE_SCRIPT="$HOME/src/ava/scripts/mlx-engine-server.sh"   # your checkout's path
```

The second line lets `ava speak` start the Kokoro server from any folder. Then do steps 4 and 5
above to check speech and dictation.

| Want | Run | Notes |
|---|---|---|
| The menu bar app | `bash AvaMenuBar/scripts/build-app.sh && open -a Ava` | Installs `/Applications/Ava.app`. To start at login: System Settings → General → Login Items |
| Claude Code speaks when it finishes or needs you | `make setup-voice-hooks`, then add the hooks to `~/.claude/settings.json` | Exact JSON in [claude-code-voice-hooks.md](claude-code-voice-hooks.md) |
| Claude Code uses Ava as a tool | `claude mcp add -s user ava -e MLX_ENGINE_SCRIPT="$HOME/src/ava/scripts/mlx-engine-server.sh" -- "$HOME/.local/bin/ava" mcp` | See [mcp.md](mcp.md) |
| Live call transcripts | `make setup-voxtral && make build` | Downloads a 2.9 GB model on first use. See [monitor.md](monitor.md) |
| A dictation hotkey (Raycast) | `make install-raycast` | Then add `~/raycast-scripts` in Raycast's settings |

**Intel Mac:** `go install github.com/iksnerd/ava/cmd/ava@latest`, then `ava setup`. It skips the
Kokoro engine there, so speech uses the macOS voice; dictation works in full.

## Updating later

- **Release install:** run the step 2 line again, then `ava setup`.
- **From source:** `git pull && make install-bin`, and rerun the app build if you use the menu
  bar app.

## Removing it

See [uninstall.md](uninstall.md).
