# Install from source

Needed for the Claude Code voice hooks and live call transcripts, to build the menu bar app
yourself, or to run on an Intel Mac. Check the machine and install Homebrew first, as steps 1 and 2
of [getting started](getting-started.md) describe.

← [Back to the README](../README.md)

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
make setup          # uv, the Kokoro Python environment (about 1.2 GB) and the 141 MB speech model
make install-bin    # builds ava and installs it to ~/.local/bin
```

Add these two lines to `~/.zshrc`, then open a new terminal window:

```bash
export PATH="$HOME/.local/bin:$PATH"
export MLX_ENGINE_SCRIPT="$HOME/src/ava/scripts/mlx-engine-server.sh"   # your checkout's path
```

The second line lets `ava speak` start the Kokoro server from any folder. Then check speech and
dictation as in step 5 of [getting started](getting-started.md#5-check-speech-and-dictation).

| Want | Run | Notes |
|---|---|---|
| The menu bar app | `bash AvaMenuBar/scripts/build-app.sh && open -a Ava` | Installs `/Applications/Ava.app`. To start at login: System Settings → General → Login Items |
| Claude Code speaks when it finishes or needs you | `make setup-voice-hooks`, then add the hooks to `~/.claude/settings.json` | Exact JSON in [claude-code-voice-hooks.md](claude-code-voice-hooks.md) |
| Claude Code uses Ava as a tool | `claude mcp add -s user ava -e MLX_ENGINE_SCRIPT="$HOME/src/ava/scripts/mlx-engine-server.sh" -- "$HOME/.local/bin/ava" mcp` | See [mcp.md](mcp.md) |
| Live call transcripts | `make setup-voxtral && make build` | Downloads a 2.9 GB model on first use. See [monitor.md](monitor.md) |
| A dictation hotkey (Raycast) | `make install-raycast` | Then add `~/raycast-scripts` in Raycast's settings |

## Intel Mac

`go install github.com/iksnerd/ava/cmd/ava@latest`, then `ava setup`. It skips the Kokoro engine
there, so speech uses the macOS voice; dictation works in full.

## Updating

`git pull && make install-bin`, and rerun the app build if you use the menu bar app.
