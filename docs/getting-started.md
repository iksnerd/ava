# Getting started on a new Mac

A checklist for setting Ava up on a Mac from nothing, written so someone other than the owner can
follow it. Every step ends with a check; don't move on until it passes.

← [Back to the README](../README.md)

**Which path?** Steps 0 to 5 install `ava` from a release: dictation, natural-voice speech,
transcribing files, and the MCP server for Claude Code. For the menu bar app with as little
Terminal as possible, see [The menu bar app](#the-menu-bar-app). The Claude Code voice hooks and
live call transcripts need the source: [Full install from source](#full-install-from-source).

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

## The menu bar app

For someone who would rather not use Terminal. Ava lives in the menu bar, and a **Set up Ava**
button in it installs what it needs. Homebrew is the one thing to install by hand first. It means
copying one line from a website into Terminal; section B walks through it.

### A. Check the Mac

Apple menu (top left) → **About This Mac**. **Chip** has to be an Apple M-series chip (M1 or later) and
**macOS** 13 or later. On an Intel Mac, the menu bar app cannot speak with the natural voice.

### B. Install Homebrew

Skip this if Homebrew is already installed. The app checks, and links you back here if it isn't.

1. Open **Terminal** (press ⌘-Space, type `Terminal`, press Return).
2. Go to [brew.sh](https://brew.sh), copy the line under "Install Homebrew", paste it into
   Terminal and press Return.
3. When it asks for your password, type your Mac login password and press Return. **Nothing
   appears as you type**; that is normal.
4. It may say it needs the Command Line Tools and install them first. The whole thing can take 10
   minutes or more. Wait until it prints **Installation successful!**
5. It ends with "Next steps" and a few lines starting with `echo`. Ignore them: Ava does not need
   them. You can close Terminal now.

### C. Install Ava

1. Download [`Ava.dmg`](https://github.com/iksnerd/ava/releases/latest/download/Ava.dmg) and open it from your Downloads folder.
2. A window opens showing the Ava icon.
   - If there is an **Applications** folder next to Ava in that window, drag Ava onto it.
   - If Ava is alone in the window, click **Finder** in the Dock to open a second window. In the
     list on its left, find **Applications**, and drag Ava from the first window onto it.
3. Open **Applications** and double-click **Ava**. macOS says it "could not verify Ava is free of
   malware", because the app is not signed. Click **Done**.
4. Open System Settings → **Privacy & Security** and scroll down to Security. Next to the message
   about Ava, click **Open Anyway**. A box pops up asking you to confirm: click **Open Anyway** in
   it too, and enter your password or use Touch ID. Ava then opens by itself (if it does not,
   double-click it in Applications again). You only do this once.

### D. Set it up

1. Once Ava is open, a waveform icon appears in the menu bar at the top right. Ava has no window
   of its own; everything happens from that icon. If you cannot see it, the menu bar
   may be too full: on a MacBook with a notch, icons that do not fit are hidden behind it. Quit an
   app you do not need in the menu bar, or hold ⌘ and drag other icons out of the way.
2. Click the waveform. A **Setup** card at the top says what is missing. Click **Set up Ava**. It
   downloads about 1.7 GB and takes a few minutes; you can close the panel meanwhile.
3. If the card says Homebrew is missing, go back to B, then click **Try again**. If it says setup
   stopped for another reason, click **Try again** once. If it stops again, copy the message it
   shows and send it to whoever pointed you to Ava, or post it as an
   [issue](https://github.com/iksnerd/ava/issues).

Check: the Setup card goes away. Click **Speak a Test Phrase** at the bottom of the panel: a
natural voice speaks. The first time takes a few seconds while the voice engine starts.

### E. Dictate

1. Click in the app you want to type into, where the text should go.
2. Click the waveform, then **Dictate**. The panel closes and Ava starts listening; nothing shows
   on screen while it does.
3. Say a sentence, then stay quiet for 2 seconds. A short pop sound means it has finished, and
   the text is pasted where you clicked.

- The first time, macOS asks whether **Ava** may use the **Microphone**. Click **Allow**.
- Pasting needs **Accessibility**. If macOS says Ava wants to control your computer, open System
  Settings → Privacy & Security → **Accessibility** and turn **Ava** on. Then dictate again.

Check: the sentence appears where your cursor was.

## Full install from source

Needed for the Claude Code voice hooks and live call transcripts, or to build the menu bar app
yourself. Do step 0 and step 1 above first.

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
