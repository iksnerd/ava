# Getting started on a new Mac

A checklist for setting Ava up on a Mac from nothing, written so someone other than the owner can
follow it. Every step ends with a check; don't move on until it passes.

← [Back to the README](../README.md)

**Which path?** This guide does the full install from a checkout: dictation, Kokoro speech, the
menu bar app, the Claude Code hooks and call transcripts. If you only need the command-line tool,
skip to [CLI only](#cli-only).

## 0. Check the machine

```bash
uname -m                 # arm64 = Apple Silicon: everything works
sw_vers -productVersion  # 13 or later for the menu bar app
```

On an Intel Mac (`x86_64`) only dictation works; speech falls back to the macOS voice, and there
is no menu bar app or call monitoring.

## 1. Developer tools and Homebrew

```bash
xcode-select --install          # git and Swift; skip if it says they are already installed
```

Install Homebrew from [brew.sh](https://brew.sh) (paste its one-line command, then run the two
`echo … >> ~/.zprofile` lines it prints at the end). Then:

```bash
brew install go gh sox whisper-cpp
```

Check: `go version`, `gh --version`, `sox --version` and `whisper-cli --help` all run.

## 2. Access to the repository

The repository is private. Your GitHub account needs access: ask the owner to add you as a
collaborator on `iksnerd/ava`, or sign in with an account that already has it.

```bash
gh auth login          # GitHub.com, HTTPS, log in with a browser
gh repo view iksnerd/ava --json name   # check: prints {"name":"ava"}, not "Could not resolve"
```

## 3. Get the code

Pick the folder now and keep it: the menu bar app and the Claude Code hooks record this path, so
moving the folder later means rebuilding the app and editing the hooks.

```bash
mkdir -p ~/src && cd ~/src
gh repo clone iksnerd/ava
cd ava
```

## 4. Install

```bash
make setup          # uv, the Kokoro Python environment (~1.2 GB) and the 141 MB speech model
make install-bin    # builds ava and installs it to ~/.local/bin
```

Add these two lines to `~/.zshrc`, then open a new terminal window:

```bash
export PATH="$HOME/.local/bin:$PATH"
export MLX_ENGINE_SCRIPT="$HOME/src/ava/scripts/mlx-engine-server.sh"   # your checkout's path
```

The second line lets `ava speak` start the Kokoro server from any folder, not only from inside
the checkout.

Check:

```bash
ava --version        # prints a version
ava engine start     # first start downloads the 339 MB Kokoro model; wait for "ready"
ava speak "hello"    # you hear a natural voice, not the robotic macOS one
```

On a slow connection the first `ava engine start` can report "failed to start in time" while the
model is still downloading; the server keeps going. Wait a minute, then `ava engine status`.

If `ava speak` prints a warning about the macOS `say` voice, the engine is not reachable: run
`ava engine status`, and see [troubleshooting](troubleshooting.md).

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
want to type into first, or use a hotkey (step 7).

## 6. The menu bar app

```bash
bash AvaMenuBar/scripts/build-app.sh    # builds and installs /Applications/Ava.app
open -a Ava
```

Check: an Ava icon appears in the menu bar. Its **Dictate** button records and pastes, and the
mute switch silences all speech. To start it at login: System Settings → General → Login Items →
add Ava.

## 7. Optional extras

| Want | Run | Notes |
|---|---|---|
| Claude Code uses Ava's voice and transcription | `claude mcp add -s user ava -e MLX_ENGINE_SCRIPT="$HOME/src/ava/scripts/mlx-engine-server.sh" -- "$HOME/.local/bin/ava" mcp` | Restart Claude Code after. See [mcp.md](mcp.md) |
| Claude Code speaks when it finishes or needs you | `make setup-voice-hooks`, then add the hooks to `~/.claude/settings.json` | Exact JSON in [claude-code-voice-hooks.md](claude-code-voice-hooks.md) |
| Live call transcripts | `make setup-voxtral && make build-ava-monitor` | Downloads a 2.9 GB model on first use. See [ava-monitor.md](ava-monitor.md) |
| A hotkey for dictation (Raycast) | `make install-raycast` | Then add `~/raycast-scripts` in Raycast's settings |

## CLI only

No checkout, no menu bar app or hooks: just `ava` and `ava-monitor` from the latest release.
Still needs step 2 (access to the private repo) and `gh`:

```bash
gh api repos/iksnerd/ava/contents/scripts/install.sh -H "Accept: application/vnd.github.raw" | bash
ava setup            # sox and whisper-cli via Homebrew, the model, and the Kokoro engine
```

Here no `MLX_ENGINE_SCRIPT` is needed: `ava setup` installs its own copy of the engine and
`ava speak` finds it. `ava-monitor` from a release still needs a checkout for call transcripts.

## Updating later

From the checkout: `git pull && make install-bin`, then rerun `bash AvaMenuBar/scripts/build-app.sh`
if the menu bar app changed. For a CLI-only install, rerun the install line and `ava setup`.

## Removing it

See [uninstall.md](uninstall.md).
