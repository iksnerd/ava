# local-whisper

Dictation, speech, and live call transcripts for macOS, running entirely
on-device.

Cloud voice tools are a non-starter for anything you wouldn't paste into a
stranger's web form. Client calls, mostly. This keeps the loop local instead:
whisper.cpp for speech-to-text, Kokoro for speech. No audio and no transcript
leaves the machine.

It started as a Go CLI for dictation and grew into the stack around it: a menu
bar app, Claude Code hooks that speak session status out loud, and an MCP server
so other programs can use the same voice. If you want a dictation app, the ones
below are better at that. This is the substrate underneath one.

## Quick start

**Dictation works on any Mac. Everything else needs Apple Silicon.** Speech,
call monitoring and the menu bar app all run through
[MLX](https://github.com/ml-explore/mlx), which has no Intel build. The menu bar
app additionally needs macOS 13 or later.

```bash
brew install sox whisper-cpp go

git clone https://github.com/iksnerd/local-whisper.git
cd local-whisper
make install-bin        # builds, downloads the 141MB model, installs to ~/.local/bin

export PATH="$HOME/.local/bin:$PATH"   # add to ~/.zshrc to make it stick

local-whisper           # speak; the text lands wherever your cursor is
```

Switch to the app you want to dictate into before running it. On a first run
your cursor is still in the terminal you just typed the command into, so your
words get pasted at your own shell prompt — that is the tool working, not
failing. A hotkey is what makes it natural: `make install-raycast` adds a
Raycast script command, or use the menu bar app's Dictate button.

**Two macOS permission prompts on first run.** Microphone, for whatever you ran
it from — miss this one and recording still "succeeds" while producing silence.
And Accessibility, because auto-paste drives Cmd+V through AppleScript;
`local-whisper --no-paste` skips it and leaves the transcript on your clipboard.

For speech and call monitoring, `make setup` does the rest: it installs `uv`,
syncs the Python environment for the Kokoro server, and downloads the model. See
[`mlx-engine/README.md`](mlx-engine/README.md) for running that server on its
own.

## What it does

**Dictate into whatever has focus.** Records until you stop talking (2s of
silence), transcribes, copies, pastes. A 20-second recording takes about 1.3s
with the default `base.en` model on an M3 Pro.

**Speak, from anywhere.** `local-whisper speak`, an MCP tool, a shell script, or
the menu bar app — all through one Kokoro server, one global mute and one
playback lock, so two of them never talk over each other. 28 voices, blendable
by passing several ids.

**Hear a web page the way a screen reader announces it.** Pair the MCP server
with [chrome-devtools MCP](https://github.com/ChromeDevTools/chrome-devtools-mcp)
and `speak_accessibility_tree` narrates a page in reading order, then reports
what only surfaces when you listen: six links that all announce "read more",
skipped heading levels, an alt text that passes the linter and reads as nonsense
out loud.

**Live call transcripts.** `voice-monitor` streams a transcript to
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
install is `make`, and most of it needs Apple Silicon. Worth it if you want to
script speech, drive it from an agent, or have a hook talk — and it is how the
one capability here with no equivalent elsewhere exists at all: narrating a web
page's accessibility tree the way a screen reader announces it.

## How it works

Four ways in: the CLI, an MCP client, the Claude Code hooks and the menu bar
app. Two engines underneath: whisper.cpp as a subprocess per transcription,
Kokoro on a local server that idles down after 15 minutes and restarts in a
couple of seconds.

The hooks deliberately don't go through the Go binary; they're bash talking to
`scripts/speak.sh`, so spoken notifications work on a machine where
`local-whisper` was never installed. One JSON config file has three independent
readers — Go, bash and Swift — held to identical answers by a contract test.

Diagram and the reasoning behind each of those choices:
[`docs/architecture.md`](docs/architecture.md).

## Privacy

No audio or transcript is ever transmitted. Both engines run locally,
`mlx-engine` binds `127.0.0.1`, and there is no account, telemetry or server
component. The only network access is downloading model weights at setup.

What lands on disk:

| What | Where | Cleaned up |
| --- | --- | --- |
| Recorded dictation audio | `/tmp/voice-input/` | On exit. A crash leaves the `.wav` behind |
| Synthesis and playback temp files | `/tmp/ava-tts-*` | After playback |
| **`voice-monitor` transcripts** | `/tmp/voice-input/transcript-<ts>.txt` | **Never**, and written world-readable |

That last row is the one to know. A call transcript stays on disk indefinitely
at mode `0644`. Pass `--log` to choose the path, and delete it when you are done.

**Accessibility permission allows synthesising any keystroke**, not just Cmd+V.
Grant it to a terminal or app you trust, or run with `--no-paste`.

**Loopback capture records the other participants.** `make setup-blackhole` sets
this up for `voice-monitor`. Recording-consent law varies by jurisdiction and
some require every party to agree; that is your call to make, not the tool's.

If you enable `llmSummary`, notification text goes to a local Ollama on
`127.0.0.1:11434` before being spoken. Still local, but a second process sees
it. Off by default.

## Common commands

```bash
local-whisper                              # dictate
local-whisper transcribe meeting.wav       # transcribe a file you already have

local-whisper speak "build finished"
git log -1 --format=%s | local-whisper speak
local-whisper speak --voice af_heart,bf_emma "a blend of two voices"
local-whisper stop                         # cancel speech from any source
local-whisper voices                       # the 28 ids --voice accepts

local-whisper engine start|status|stop     # the Kokoro server
local-whisper mcp                          # serve the stack over MCP
local-whisper a11y snapshot.txt            # narrate an accessibility tree
```

Every command and flag: [`docs/cli.md`](docs/cli.md).

## Documentation

Start here:

| | |
| --- | --- |
| [`docs/cli.md`](docs/cli.md) | Every command and flag, context files |
| [`docs/troubleshooting.md`](docs/troubleshooting.md) | When nothing pastes, or nothing plays |
| [`docs/tuning.md`](docs/tuning.md) | What the engines really expose: Kokoro's five parameters, inline stress and IPA markup, whisper.cpp flags |

By component:
[`docs/mcp.md`](docs/mcp.md) (the five MCP tools) ·
[`docs/voice-monitor.md`](docs/voice-monitor.md) (live transcripts, loopback setup) ·
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
make build                 # binary to bin/local-whisper
make test                  # Go, mlx-engine, voxtral, and the voice hooks
make lint                  # vet, format check, ruff, hardcoded-path check
make check-swift-config    # the Swift config reader vs bash and Go (needs swiftc)
```

`go run ./cmd/local-whisper [flags]` runs without building. Read
[`CONTRIBUTING.md`](CONTRIBUTING.md) before changing the config readers or the
speech protocol.

## License

MIT. Third-party components and model licences are in [`NOTICE`](NOTICE). Note
that `voice-monitor --diarize` downloads a model licensed for non-commercial use
only.
