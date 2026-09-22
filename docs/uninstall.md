# Uninstalling

What each install path leaves on disk, and how to remove it. Every path below
was checked against the code that writes it.

← [Back to the README](../README.md)

## The quick part

```bash
make uninstall
```

From a checkout, this stops the Kokoro server and deletes what only this
project uses:

- `~/.local/bin/ava` and `~/.local/bin/ava-monitor`
- `~/raycast-scripts/whisper-transcribe.sh`
- `~/Library/Application Support/ava/engine/`, the engine bundle and its
  ~1.2 GB Python environment that `ava setup` installs

It leaves the rest to you, because other tools may share it or you may want to
keep it. It prints the same list as below when it is done.

Without a checkout, delete those paths by hand; stop the server first with
`ava engine stop`.

## The rest

### Models

```bash
rm ~/.local/share/whisper-cpp/ggml-base.en.bin   # 141 MB
rm ~/.local/share/whisper-cpp/ggml-tiny.en.bin   # 74 MB, if you fetched it
```

`~/.local/share/whisper-cpp/` is whisper.cpp's conventional location, not this
project's, so another whisper.cpp tool may be using the same files.

Kokoro and Voxtral live in the shared Hugging Face cache, which other tools also
read, so `make uninstall` never touches it:

```bash
rm -rf ~/.cache/huggingface/hub/models--mlx-community--Kokoro-82M-bf16                    # 339 MB
rm -rf ~/.cache/huggingface/hub/models--mlx-community--Voxtral-Mini-4B-Realtime-2602-4bit # 2.9 GB, only if you ran ava-monitor
```

### The menu bar app

```bash
rm -rf /Applications/Ava.app
rm -rf ~/Library/Application\ Support/ava   # config.json: mute, voice, speed, volume
```

Its two Services, **Read Aloud with Ava** and **Toggle Ava Mute**, are
declared inside the app bundle, so they go with it. If they linger in the
Services menu, run `/System/Library/CoreServices/pbs -flush` or log out and
back in. The CLI reads the same `config.json`, so delete it only once both are
gone.

### Claude Code

If you added the voice hooks, remove the `Notification` and `Stop` entries
pointing at `scripts/hook-notify.sh` and `scripts/hook-stop.sh` from
`~/.claude/settings.json`. `make setup-voice-hooks` only builds their Python
environment inside the checkout, so it wrote nothing there itself.

If you registered the MCP server:

```bash
claude mcp remove -s user ava
```

### Permissions

System Settings → Privacy & Security → **Microphone** and **Accessibility**.
The grants belong to whatever ran the command (Terminal, Raycast, your editor,
or Ava.app), not to `ava`, so remove only the ones you gave for this.

### Dependencies

All general-purpose, so remove them only if nothing else needs them:

- `sox` and `whisper-cpp`: `brew uninstall sox whisper-cpp`
- `uv`, installed by `make setup-deps` with Astral's installer rather than
  Homebrew: `uv self uninstall`
- BlackHole and `switchaudio-osx`, if `make setup-blackhole` installed them for
  call capture: `brew uninstall --cask blackhole-2ch` and
  `brew uninstall switchaudio-osx`

### The checkout

Deleting the repo removes everything else: `bin/`, and the virtualenvs under
`mlx-engine/`, `voxtral/` and `scripts/voice_hooks/`.
