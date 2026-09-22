# Ava

Dictation, speech, and live call transcripts for macOS, running entirely
on-device.

Formerly `local-whisper`. The commands are now `ava` and `ava-monitor`; the old
name still works as an alias until 0.7.0.

Cloud voice tools are a non-starter for anything you wouldn't paste into a
stranger's web form. Client calls, mostly. This keeps the loop local instead:
whisper.cpp for speech-to-text, Kokoro for speech. No audio and no transcript
leaves the machine.

It started as a Go CLI for dictation and grew into the stack around it: a menu
bar app, Claude Code hooks that speak session status out loud, and an
MCP server so other programs can use the same voice. If you want a dictation
app, the ones below are better at that. This is the substrate underneath one.

## Install

**Needs** a Mac with Apple Silicon (M1 or later) and [Homebrew](https://brew.sh). The menu bar
app also needs macOS 13 or later. On an Intel Mac only dictation works: see
[From source](#from-source).

### From a release

```bash
curl -fsSL https://raw.githubusercontent.com/iksnerd/ava/main/scripts/install.sh | bash
ava setup
```

- The first line downloads the latest [release](https://github.com/iksnerd/ava/releases),
  checks it against its published checksum, and installs `ava` and `ava-monitor` to
  `~/.local/bin`. If that folder is not on your PATH it prints the line to add to `~/.zshrc`.
- `ava setup` installs what the binary needs: `sox` and `whisper-cli` through Homebrew, the
  141 MB speech model, and the Kokoro voice engine (about 1.2 GB; `ava setup --skip-engine` for
  dictation only). Re-running it is cheap: finished steps are skipped.

Then check it works:

```bash
ava speak "hello"   # a natural voice; the first run downloads the 339 MB voice model
ava                 # dictate: speak, pause for 2 seconds, the text is pasted at your cursor
```

A specific version: `curl -fsSL https://raw.githubusercontent.com/iksnerd/ava/main/scripts/install.sh | bash -s v0.6.2`.

**Downloading from the Releases page instead:** the binaries are unsigned (there is no paid Apple
Developer account behind them), so macOS quarantines anything a browser downloads, and a
quarantined unsigned binary is killed without a message. After extracting the `.tar.gz`, check
it and clear the flag:

```bash
shasum -a 256 -c checksums.txt --ignore-missing
xattr -d com.apple.quarantine ava ava-monitor
mv ava ava-monitor ~/.local/bin/
```

A release install gives you `ava`: dictation, `speak`, `transcribe`, the MCP server and the
accessibility narrator. The menu bar app, the Claude Code voice hooks and `ava-monitor`'s call
transcripts need the source.

Step by step with a check after each, for a new Mac or someone else's:
[`docs/getting-started.md`](docs/getting-started.md).

### From source

```bash
brew install sox whisper-cpp go

git clone https://github.com/iksnerd/ava.git
cd ava
make setup              # uv, the Kokoro engine's Python environment, the speech model
make install-bin        # builds ava and installs it to ~/.local/bin

export PATH="$HOME/.local/bin:$PATH"                             # add both to ~/.zshrc
export MLX_ENGINE_SCRIPT="$PWD/scripts/mlx-engine-server.sh"     # lets `ava speak` start the engine from any folder

ava                     # speak; the text lands wherever your cursor is
```

On an Intel Mac, `go install github.com/iksnerd/ava/cmd/ava@latest` then `ava setup`, which
skips the Kokoro engine there; speech uses the macOS voice.

### First run

Switch to the app you want to dictate into before running it. On a first run
your cursor is still in the terminal you just typed the command into, so your
words get pasted at your own shell prompt — that is the tool working, not
failing. A hotkey is what makes it natural: `make install-raycast` adds a
Raycast script command, or use the menu bar app's Dictate button.

**Two macOS permission prompts on first run.** Microphone, for whatever you ran
it from — miss this one and recording still "succeeds" while producing silence.
And Accessibility, because auto-paste drives Cmd+V through AppleScript;
`ava --no-paste` skips it and leaves the transcript on your clipboard.

### From source: the menu bar app and call transcripts

Call monitoring needs `make setup-voxtral` and `make build-ava-monitor`; see
[`docs/ava-monitor.md`](docs/ava-monitor.md). The Kokoro server can also run on its own: see
[`mlx-engine/README.md`](mlx-engine/README.md).

The menu bar app is built from the checkout and installed to
`/Applications/Ava.app`:

```bash
bash AvaMenuBar/scripts/build-app.sh
open -a Ava
```

It does not start itself at login; add it under System Settings → General →
Login Items if you want it there. Details in
[`AvaMenuBar/README.md`](AvaMenuBar/README.md).

## What it does

**Dictate into whatever has focus.** Records until you stop talking (2s of
silence), transcribes, copies, pastes. A 20-second recording takes about 1.3s
with the default `base.en` model on an M3 Pro.

**Speak, from anywhere.** `ava speak`, an MCP tool, a shell script, or
the menu bar app — all through one Kokoro server, one global mute and one
playback lock, so two of them never talk over each other. 28 voices, blendable
by passing several ids.

**Hear a web page the way a screen reader announces it.** Pair the MCP server
with [chrome-devtools MCP](https://github.com/ChromeDevTools/chrome-devtools-mcp)
and `speak_accessibility_tree` narrates a page in reading order, then reports
what only surfaces when you listen: six links that all announce "read more",
skipped heading levels, an alt text that passes the linter and reads as nonsense
out loud.

**Live call transcripts.** `ava-monitor` streams a transcript to
`localhost:8766` from your mic or a loopback device, with optional speaker
labels. This is the one place Voxtral still runs: it is built for streaming,
where this project's whisper path transcribes a complete file per subprocess.
(whisper.cpp does ship a streaming example; it just isn't what `pkg/stt/whisper`
wraps.)

**Spoken Claude Code notifications.** Hooks that say when Claude finishes a turn
or needs a decision, optionally shortened by a local Ollama model first. They are
plain bash, so they work on a machine where the Go binary was never installed.

## If you are comparing it to a dictation app

You probably should use a dictation app. [VoiceInk](https://github.com/beingpax/VoiceInk)
is open source, has a real installer and a large user base;
[Handy](https://github.com/cjpais/Handy) is cross-platform and states its scope
perfectly — "one tool, one job";
[superwhisper](https://superwhisper.com) and [MacWhisper](https://goodsnooze.gumroad.com/l/macwhisper)
are polished commercial products. All of them run locally too. Privacy is not
the difference between us.

The difference is shape. Those are applications: the app is the product, and
dictation is what it does. Here the *protocol* is the product. A CLI, a bash
hook, an MCP client and a menu bar app are four peers that coordinate through
the filesystem — one JSON config with three independent readers in Go, bash and
Swift, one activity marker, one exclusive `flock(2)`. Nothing owns speech. The
Claude Code hooks deliberately don't call the Go binary, so spoken notifications
work on a machine where it was never installed.

That buys composability and costs convenience. There is no signed `.dmg`, the
install is a shell script or `make`, and most of it needs Apple Silicon. Worth
it if you want to script speech, drive it from an agent, or have a hook talk —
and it is how the one capability here with no equivalent elsewhere exists at
all: narrating a web page's accessibility tree the way a screen reader announces
it.

## How it works

Four ways in: the CLI, an MCP client, the Claude Code hooks and the menu bar
app. Two engines underneath: whisper.cpp as a subprocess per transcription,
Kokoro on a local server that idles down after 15 minutes and restarts in a
couple of seconds.

The hooks deliberately don't go through the Go binary; they're bash talking to
`scripts/speak.sh`, so spoken notifications work on a machine where
`ava` was never installed. One JSON config file has three independent
readers — Go, bash and Swift — held to identical answers by a contract test.

Diagram and the reasoning behind each of those choices:
[`docs/architecture.md`](docs/architecture.md).

## Privacy

No audio or transcript is ever transmitted. Both engines run locally,
`mlx-engine` and `ava-monitor` bind `127.0.0.1`, and there is no account,
telemetry or server component. The only network access is installing
dependencies and downloading model weights, at setup or the first time a model
loads.

What lands on disk:

| What | Where | Cleaned up |
| --- | --- | --- |
| Recorded dictation audio | `/tmp/voice-input/` | On exit. A crash leaves the `.wav` behind |
| Synthesis and playback temp files | `/tmp/ava-tts-*` | After playback |
| **`ava-monitor` transcripts** | `/tmp/voice-input/transcript-<ts>.txt` | **Never**, and written world-readable |

That last row is the one to know. A call transcript stays on disk indefinitely
at mode `0644`. Pass `--log` to choose the path, and delete it when you are done.

**Accessibility permission allows synthesising any keystroke**, not just Cmd+V.
Grant it to a terminal or app you trust, or run with `--no-paste`.

**Loopback capture records the other participants.** `make setup-blackhole` sets
this up for `ava-monitor`. Recording-consent law varies by jurisdiction and
some require every party to agree; that is your call to make, not the tool's.

If you enable `llmSummary`, notification text goes to a local Ollama on
`127.0.0.1:11434` before being spoken. Still local, but a second process sees
it. Off by default.

## Common commands

```bash
ava                              # dictate
ava transcribe meeting.wav       # transcribe a file you already have

ava speak "build finished"
git log -1 --format=%s | ava speak
ava speak --voice af_heart,bf_emma "a blend of two voices"
ava stop                         # cancel speech from any source
ava voices                       # the 28 ids --voice accepts

ava engine start|status|stop     # the Kokoro server
ava mcp                          # serve the stack over MCP
ava a11y snapshot.txt            # narrate an accessibility tree
```

Every command and flag: [`docs/cli.md`](docs/cli.md).

## Documentation

Start here:

| | |
| --- | --- |
| [`docs/getting-started.md`](docs/getting-started.md) | Setting up a new Mac from nothing, step by step |
| [`docs/cli.md`](docs/cli.md) | Every command and flag, context files |
| [`docs/troubleshooting.md`](docs/troubleshooting.md) | When nothing pastes, or nothing plays |
| [`docs/tuning.md`](docs/tuning.md) | What the engines really expose: Kokoro's five parameters, inline stress and IPA markup, whisper.cpp flags |
| [`docs/uninstall.md`](docs/uninstall.md) | Everything an install leaves on disk, and `make uninstall` |

By component:
[`docs/mcp.md`](docs/mcp.md) (the five MCP tools) ·
[`docs/ava-monitor.md`](docs/ava-monitor.md) (live transcripts, loopback setup) ·
[`docs/claude-code-voice-hooks.md`](docs/claude-code-voice-hooks.md) (spoken notifications, every config key) ·
[`mlx-engine/README.md`](mlx-engine/README.md) (the Kokoro server) ·
[`AvaMenuBar/README.md`](AvaMenuBar/README.md) (the menu bar app)

Working on it:
[`docs/architecture.md`](docs/architecture.md) ·
[`CONTRIBUTING.md`](CONTRIBUTING.md) ·
[`AGENTS.md`](AGENTS.md) ·
[`SECURITY.md`](SECURITY.md) ·
[`NOTICE`](NOTICE)

## Development

```bash
make install-hooks         # pre-commit checks; CI only runs on version tags
make build                 # binary to bin/ava
make test                  # Go, mlx-engine, voxtral, and the voice hooks
make lint                  # vet, format check, ruff, and the repo's drift checks
make check-swift-config    # the Swift config reader vs bash and Go (needs swiftc)
```

`go run ./cmd/ava [flags]` runs without building. Read
[`CONTRIBUTING.md`](CONTRIBUTING.md) before changing the config readers or the
speech protocol.

## License

MIT. Third-party components and model licences are in [`NOTICE`](NOTICE). Note
that `ava-monitor --diarize` downloads a model licensed for non-commercial use
only.
