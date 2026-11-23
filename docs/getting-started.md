# Getting started

Install `ava` from a release on an Apple Silicon Mac: dictation, natural-voice speech, file
transcription and the MCP server for Claude Code. Each step ends with a check.

← [Back to the README](../README.md)

Other paths: for the menu bar app with no Terminal, see [the menu bar app](menu-bar-app.md). For
the Claude Code voice hooks, live call transcripts or an Intel Mac, see
[install from source](install-from-source.md).

## 1. Check the machine

```bash
uname -m                 # arm64 means Apple Silicon
sw_vers -productVersion  # the menu bar app needs macOS 13 or later
```

Release binaries are Apple Silicon only, and `install.sh` stops on anything else. The CLI itself
declares no minimum macOS version; the installer checks only for Apple Silicon. On an Intel Mac,
see [the Intel note](install-from-source.md#intel-mac); only dictation works there.

## 2. Homebrew

`ava setup` installs its tools through [Homebrew](https://brew.sh). If `brew --version` does not
run, install Homebrew from brew.sh: paste its one-line command, run the `echo … >> ~/.zprofile`
lines it prints at the end, and open a new terminal window.

Check: `brew --version` prints a version.

## 3. Install ava

```bash
curl -fsSL https://raw.githubusercontent.com/iksnerd/ava/main/scripts/install.sh | bash
```

This downloads the latest release, checks it against the published checksum, and installs `ava` to
`~/.local/bin`. If it prints a line starting `echo 'export PATH=`, run that line and open a new
terminal window.

Check: `ava --version` prints a version.

To install a specific release, pass its tag as the first argument. `bash -s` makes `bash` read the
script from the pipe and hand it the arguments after it. Replace `vX.Y.Z` with a tag from the
[Releases page](https://github.com/iksnerd/ava/releases); `install.sh` accepts 0.6.0 or later:

```bash
curl -fsSL https://raw.githubusercontent.com/iksnerd/ava/main/scripts/install.sh | bash -s vX.Y.Z
```

**Downloading from the Releases page instead.** The binaries are unsigned (there is no paid Apple
Developer account behind them), so macOS quarantines anything a browser downloads and kills a
quarantined unsigned binary without a message. After extracting the `.tar.gz`, in that folder:

```bash
shasum -a 256 -c checksums.txt --ignore-missing
xattr -d com.apple.quarantine ava
mkdir -p ~/.local/bin && mv ava ~/.local/bin/
```

## 4. Install what ava needs

```bash
ava setup
```

This installs `sox`, `whisper-cli` and `uv` through Homebrew, then downloads about 1.7 GB: the
141 MB speech model, about 1.2 GB for the Kokoro engine's Python environment and the 339 MB Kokoro
model with every voice. It takes a few minutes the first time. After it, speech needs no network.
Every step is skipped when already done, so if anything fails, fix it and run `ava setup` again.

Check: it ends with `✅ Setup complete.`

## 5. Check speech and dictation

```bash
ava speak "hello"
ava --no-paste       # say a sentence, then stay quiet for 2 seconds
```

- The first `ava speak` starts the engine, which takes a few seconds. You should hear a natural
  voice. If it prints a warning about the macOS `say` voice instead, run `ava engine start`, wait
  for "ready", and try again.
- macOS asks for **Microphone** access for your terminal app. Allow it. If it never asked and
  nothing was transcribed, add the terminal under System Settings → Privacy & Security →
  Microphone.
- Run plain `ava` once. Pasting needs **Accessibility**: allow it for the terminal under System
  Settings → Privacy & Security → Accessibility.

Check: a natural voice says "hello", and the transcript from `ava` lands at the cursor. On this
first run the cursor is in the terminal, so the words appear at your shell prompt. Normally you
switch to the app you want to type into first, then run `ava`.

## Next

- **Claude Code uses Ava's voice and transcription:**
  `claude mcp add -s user ava -- "$HOME/.local/bin/ava" mcp`, then restart Claude Code. See
  [mcp.md](mcp.md).
- **Every command and flag:** `ava --help`, or [cli.md](cli.md).

## Updating later

- **Release install:** `ava update`, then `ava setup` only if the release notes say something
  changed that needs it.
- **From source:** `git pull && make install-bin`, and rebuild the app if you use the menu bar app.

## Removing it

See [uninstall.md](uninstall.md).
