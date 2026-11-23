# Ava

On-device dictation, speech and live call transcripts for macOS. No audio and no transcript leaves the machine.

[Getting started](docs/getting-started.md) · [CLI reference](docs/cli.md) · [MCP tools](docs/mcp.md) · [Architecture](docs/architecture.md)

Ava is a Go CLI built on [whisper.cpp](https://github.com/ggml-org/whisper.cpp) for speech-to-text and [Kokoro](https://huggingface.co/mlx-community/Kokoro-82M-bf16) for text-to-speech. The CLI, an MCP server, Claude Code hooks and a menu bar app share one voice engine, one config file and one playback lock, so you can script speech, drive it from an agent, or have a hook talk.

## Install

Needs an Apple Silicon Mac and [Homebrew](https://brew.sh).

```bash
curl -fsSL https://raw.githubusercontent.com/iksnerd/ava/main/scripts/install.sh | bash
ava setup            # sox, whisper-cli, uv, the speech model and the Kokoro voice (about 1.7 GB)
ava speak "hello"    # the first run starts the engine, a few seconds
ava                  # dictate: speak, pause for 2 seconds, the text is pasted at your cursor
```

macOS asks for Microphone access (for the app you ran it from) and Accessibility (to paste; `--no-paste` leaves the text on the clipboard). `ava setup --skip-engine` installs dictation only.

Prefer no Terminal? Download [`Ava.dmg`](https://github.com/iksnerd/ava/releases/latest/download/Ava.dmg), drag it to Applications, and click **Set up Ava** in the menu bar. It is unsigned, so macOS asks you to confirm once. The menu bar steps are in [The menu bar app](docs/menu-bar-app.md), a pinned version in [Getting started](docs/getting-started.md), and building from source in [Install from source](docs/install-from-source.md).

## What it does

- **Dictation.** Records until 2 seconds of silence, transcribes, pastes at the cursor. About 1.3 s for 20 seconds of audio with the default `base.en` model on an M3 Pro.
- **Speech.** `ava speak`, the MCP server, the hooks and the menu bar all use one Kokoro server, one global mute and one playback lock, so two never talk over each other. 28 voices, blendable.
- **Transcription.** `ava transcribe file.wav`, with text, SRT, WebVTT or JSON output.
- **Page narration.** With [chrome-devtools MCP](https://github.com/ChromeDevTools/chrome-devtools-mcp), `speak_accessibility_tree` reads a page the way a screen reader does and reports what only shows up when you listen, such as repeated "read more" links or skipped heading levels.
- **Live call transcripts.** `ava monitor` streams a transcript to `localhost:8766` from the mic or a loopback device, optionally with speaker labels ([docs](docs/monitor.md)).
- **Spoken Claude Code notifications.** Hooks say when a turn finishes or a decision is needed ([docs](docs/claude-code-voice-hooks.md)).

```bash
ava transcribe meeting.wav
git log -1 --format=%s | ava speak
ava speak --voice af_heart,bf_emma "a blend of two voices"
ava stop              # cancel speech from any source
ava engine status     # start | status | stop for the Kokoro server
ava mcp               # serve the stack over MCP (stdio)
ava update            # install the latest release
```

Every command and flag: [`docs/cli.md`](docs/cli.md).

## When not to use it

- You want a dictation app. [VoiceInk](https://github.com/beingpax/VoiceInk) (open source), [Handy](https://github.com/cjpais/Handy) (cross-platform), [superwhisper](https://superwhisper.com) and [MacWhisper](https://goodsnooze.gumroad.com/l/macwhisper) are more polished at that, and all run locally. Ava is the layer underneath one.
- You are on Intel, Linux or Windows. Only dictation works on Intel Macs; speech needs Apple Silicon (MLX).
- You need a signed installer. The binary and the `.dmg` are unsigned.

## How it works

Four front ends (CLI, MCP, hooks, menu bar) and two engines: whisper.cpp as a subprocess per transcription, and Kokoro on a local server that idles down after 15 minutes and restarts in a couple of seconds. Everything that speaks goes through one Go implementation. See [`docs/architecture.md`](docs/architecture.md).

## Privacy

Both engines run locally, `mlx-engine` and `ava monitor` bind `127.0.0.1`, and there is no account, telemetry or server component. The network is used only to install dependencies and download model weights.

| What | Where | Cleaned up |
| --- | --- | --- |
| Dictation audio | `/tmp/voice-input/` | On exit; a crash leaves the `.wav` behind |
| Speech audio | `$TMPDIR/ava-tts-*` | After playback |
| **`ava monitor` transcripts** | `/tmp/voice-input/transcript-<ts>.txt` | **Never**, and readable by every user (`0644`) |

Use `ava monitor --log` to choose where a call transcript goes, and delete it when you are done. Accessibility permission allows any keystroke, so grant it only to a terminal or app you trust. Loopback capture records the other participants; consent law varies. With `llmSummary` on (off by default), hook text goes to a local Ollama on `127.0.0.1:11434` before it is spoken.

## Documentation

[Getting started](docs/getting-started.md) · [Menu bar app guide](docs/menu-bar-app.md) · [Install from source](docs/install-from-source.md) · [CLI](docs/cli.md) · [Troubleshooting](docs/troubleshooting.md) · [Tuning](docs/tuning.md) · [MCP](docs/mcp.md) · [Engine API](docs/engine-api.md) · [Monitor](docs/monitor.md) · [Voice hooks](docs/claude-code-voice-hooks.md) · [Uninstall](docs/uninstall.md) · [Architecture](docs/architecture.md) · [Menu bar app](AvaMenuBar/README.md) · [Kokoro server](mlx-engine/README.md)

## Contributing

`make install-hooks`, then `make lint && make test` before a commit. CI runs only on version tags, so a pull request gets no automated check; say in it what you ran. See [`CONTRIBUTING.md`](CONTRIBUTING.md). Security reports: [`SECURITY.md`](SECURITY.md).

## License

MIT. Third-party components and model licences are in [`NOTICE`](NOTICE). `ava monitor --diarize` downloads a model licensed for non-commercial use only.
