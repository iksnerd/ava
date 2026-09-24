# Ava

Dictation, speech, and live call transcripts for macOS, running entirely
on-device.

Cloud voice tools are a non-starter for anything you wouldn't paste into a
stranger's web form. Client calls, mostly. This keeps the loop local:
whisper.cpp for speech-to-text, Kokoro for speech. No audio and no transcript
leaves the machine.

It started as a Go CLI for dictation and grew into the stack around it: a menu
bar app, Claude Code hooks that speak session status out loud, and an MCP
server so other programs can use the same voice. If you want a dictation app,
the ones [below](#compared-with-a-dictation-app) are better at that. This is
the substrate underneath one.

## Install

Needs a Mac with Apple Silicon and [Homebrew](https://brew.sh); the menu bar app
also needs macOS 13 or later.

```bash
curl -fsSL https://raw.githubusercontent.com/iksnerd/ava/main/scripts/install.sh | bash
ava setup
```

The first line installs the latest [release](https://github.com/iksnerd/ava/releases)
to `~/.local/bin` after checking its checksum. `ava setup` installs `sox`,
`whisper-cli` and `uv` through Homebrew, the 141 MB speech model, and the Kokoro voice
engine with its 339 MB model and every voice (about 1.5 GB in all;
`--skip-engine` for dictation only), so speech works offline afterwards. Then:

```bash
ava speak "hello"   # the first run starts the engine, a few seconds
ava                 # dictate: speak, pause for 2 seconds, the text is pasted at your cursor
```

A release gives you dictation, `speak`, `transcribe`, the MCP server and the
accessibility narrator. A pinned version, a manual download from the Releases
page, and a check after every step are in
[`docs/getting-started.md`](docs/getting-started.md).

### From source

The menu bar app, the Claude Code voice hooks and `ava monitor`'s call
transcripts need a checkout:

```bash
brew install sox whisper-cpp go
git clone https://github.com/iksnerd/ava.git && cd ava
make setup                # uv, the Kokoro engine's Python environment, the speech model
make install-bin          # builds ava and installs it to ~/.local/bin

export PATH="$HOME/.local/bin:$PATH"                          # add both to ~/.zshrc
export MLX_ENGINE_SCRIPT="$PWD/scripts/mlx-engine-server.sh"  # optional: lets `ava speak` start this checkout's engine from any folder
```

- Menu bar app: `bash AvaMenuBar/scripts/build-app.sh`, then `open -a Ava`
  ([`AvaMenuBar/README.md`](AvaMenuBar/README.md)).
- Voice hooks: [`docs/claude-code-voice-hooks.md`](docs/claude-code-voice-hooks.md).
  They speak through `ava`, so keep a built or installed one around.
- Call transcripts: `make setup-voxtral`, then `bin/ava monitor`
  ([`docs/monitor.md`](docs/monitor.md)).

On an Intel Mac only dictation works: `go install github.com/iksnerd/ava/cmd/ava@latest`,
then `ava setup`, which skips Kokoro there and speaks with the macOS voice.

### First run

Switch to the app you want to dictate into first. Run from a terminal, the text
is pasted at your own shell prompt, which is the tool working. A hotkey makes it
natural: `make install-raycast` adds a Raycast command, or use the menu bar app.

macOS asks for two permissions. **Microphone**, for whatever you ran it from:
without it, recording "succeeds" and captures silence. **Accessibility**,
because auto-paste sends Cmd+V through AppleScript; `ava --no-paste` leaves the
transcript on the clipboard instead.

## What it does

**Dictate into whatever has focus.** Records until 2 seconds of silence,
transcribes, copies, pastes. A 20-second recording takes about 1.3 s with the
default `base.en` model on an M3 Pro.

**Speak, from anywhere.** `ava speak`, an MCP tool, the Claude Code hooks and
the menu bar app all go through one Kokoro server, one global mute and one
playback lock, so two of them never talk over each other. 28 voices, blendable
by passing several ids.

**Hear a web page the way a screen reader announces it.** With
[chrome-devtools MCP](https://github.com/ChromeDevTools/chrome-devtools-mcp),
`speak_accessibility_tree` narrates a page in reading order, then reports what
only shows up when you listen: six links that all say "read more", skipped
heading levels, alt text that passes the linter and sounds like nonsense.

**Live call transcripts.** `ava monitor` streams a transcript to
`localhost:8766` from your mic or a loopback device, with optional speaker
labels. It uses Voxtral, which is built for streaming; dictation's whisper.cpp
path transcribes a complete file at a time.

**Spoken Claude Code notifications.** Hooks that say when Claude finishes a
turn or needs a decision, optionally shortened by a local Ollama model first.

## Compared with a dictation app

You probably want one. [VoiceInk](https://github.com/beingpax/VoiceInk) is open
source with a real installer, [Handy](https://github.com/cjpais/Handy) is
cross-platform, and [superwhisper](https://superwhisper.com) and
[MacWhisper](https://goodsnooze.gumroad.com/l/macwhisper) are polished
commercial products. All of them run locally too.

The difference is shape. There the app is the product. Here the pieces are
peers: a CLI, an MCP server, shell hooks and a menu bar app share one config
file, one activity marker and one `flock(2)`, so you can script speech, drive it
from an agent, or have a hook talk. The cost is convenience: no signed `.dmg`,
an install that is a shell script or `make`, and Apple Silicon for most of it.

## How it works

Four ways in (the CLI, an MCP client, the Claude Code hooks, the menu bar app)
and two engines underneath: whisper.cpp as a subprocess per transcription, and
Kokoro on a local server that idles down after 15 minutes and restarts in a
couple of seconds. Everything that speaks goes through one Go implementation;
the hooks and the menu bar reach it through `scripts/speak.sh`.

Diagram and the reasoning behind each choice:
[`docs/architecture.md`](docs/architecture.md).

## Privacy

No audio or transcript is ever transmitted. Both engines run locally,
`mlx-engine` and `ava monitor` bind `127.0.0.1`, and there is no account,
telemetry or server component. The network is used only to install
dependencies and download model weights.

What lands on disk:

| What | Where | Cleaned up |
| --- | --- | --- |
| Dictation audio | `/tmp/voice-input/` | On exit; a crash leaves the `.wav` behind |
| Speech audio | `$TMPDIR/ava-tts-*` (your per-user temp folder) | After playback |
| **`ava monitor` transcripts** | `/tmp/voice-input/transcript-<ts>.txt` | **Never**, and readable by every user (`0644`) |

Pass `ava monitor --log` to choose where a call transcript goes, and delete it
when you are done.

**Accessibility permission allows any keystroke**, not just Cmd+V. Grant it to a
terminal or app you trust, or use `--no-paste`.

**Loopback capture records the other participants.** Consent law varies, and
some places require every party to agree. That is your call, not the tool's.

With `llmSummary` on (off by default), hook text goes to a local Ollama on
`127.0.0.1:11434` before it is spoken.

## Common commands

```bash
ava                              # dictate
ava transcribe meeting.wav       # transcribe a file you already have

ava speak "build finished"
git log -1 --format=%s | ava speak
ava speak --voice af_heart,bf_emma "a blend of two voices"
ava stop                         # cancel speech from any source
ava voices                       # the ids --voice accepts

ava engine start|status|stop     # the Kokoro server
ava mcp                          # serve the stack over MCP
ava a11y snapshot.txt            # narrate an accessibility tree
```

Every command and flag: [`docs/cli.md`](docs/cli.md).

## Documentation

| | |
| --- | --- |
| [`docs/getting-started.md`](docs/getting-started.md) | A new Mac from nothing, with a check after every step |
| [`docs/cli.md`](docs/cli.md) | Every command and flag, context files |
| [`docs/troubleshooting.md`](docs/troubleshooting.md) | When nothing pastes, or nothing plays |
| [`docs/tuning.md`](docs/tuning.md) | Kokoro's parameters, stress and IPA markup, whisper.cpp flags |
| [`docs/mcp.md`](docs/mcp.md) | The five MCP tools |
| [`docs/monitor.md`](docs/monitor.md) | Live transcripts and loopback setup |
| [`docs/claude-code-voice-hooks.md`](docs/claude-code-voice-hooks.md) | Spoken notifications and every config key |
| [`docs/uninstall.md`](docs/uninstall.md) | Everything an install leaves on disk |
| [`docs/architecture.md`](docs/architecture.md) | How the pieces fit, and why |

Also: [`mlx-engine/README.md`](mlx-engine/README.md) (the Kokoro server),
[`AvaMenuBar/README.md`](AvaMenuBar/README.md) (the menu bar app).

## Contributing

`make install-hooks`, then `make lint && make test` before a commit. CI runs
only on version tags, so a pull request gets no automated check; say in it what
you ran. [`CONTRIBUTING.md`](CONTRIBUTING.md) covers the rest,
including what to read before touching the config readers or the speech
protocol. Security reports: [`SECURITY.md`](SECURITY.md).

## License

MIT. Third-party components and model licences are in [`NOTICE`](NOTICE).
`ava monitor --diarize` downloads a model licensed for non-commercial use only.
