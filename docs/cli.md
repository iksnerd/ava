# CLI reference

Every command `local-whisper` accepts. For the MCP server see
[`mcp.md`](mcp.md); for `voice-monitor` see [`voice-monitor.md`](voice-monitor.md).

← [Back to the README](../README.md)

## Dictate

The bare command records, transcribes, copies, and pastes.

```bash
local-whisper
```

| Flag | Default | Does |
|---|---|---|
| `--engine` | `whisper` | `whisper` (subprocess) or `voxtral` (local server) |
| `--model` | `base` | `base` (141MB) or `tiny` (74MB, faster, less accurate). whisper engine only |
| `--lang` | `en` | Language code: `en`, `es`, `fr`, `de`, … |
| `--context` | — | Path to a context file (whisper engine only, see below) |
| `--output` | — | Also write the transcript to this file |
| `--dir` | — | Change to this directory first |
| `--no-paste` | off | Copy to the clipboard but don't paste |
| `--no-sound` | off | No audio cues |
| `--verbose` | `true` | `--verbose=false` silences status messages |

```bash
local-whisper --dir ~/projects/app --lang en --model base --output transcript.txt
```

## Speak

Kokoro through the local server, falling back to macOS `say` when it's down.
Goes through the same mute switch, activity markers and playback lock as the
voice hooks, so speech from different sources queues instead of overlapping.

```bash
local-whisper speak "build finished"
git log -1 --format=%s | local-whisper speak     # no arguments: reads stdin
local-whisper speak --voice bf_emma --speed 0.9 "slower, british"
local-whisper speak --async "don't wait for playback"
```

| Flag | Does |
|---|---|
| `--voice` | A voice id from `local-whisper voices`; two comma-separated ids blend them |
| `--speed` | Rate multiplier, overriding the configured one |
| `--async` | Return as soon as playback starts |
| `--server-url` | Point at an mlx-engine somewhere other than `127.0.0.1:8765` |

While the menu bar app's global Mute is on, `speak` prints a notice and plays
nothing rather than reporting success.

```bash
local-whisper stop       # cancel speech from any source, including the hooks
local-whisper voices     # ids, accents, and which one is current
```

## Transcribe a file

```bash
local-whisper transcribe meeting.wav
local-whisper transcribe --engine voxtral --lang es clip.wav
local-whisper transcribe --output notes.txt meeting.wav
```

Takes 16kHz mono WAV. Same `--engine`, `--model` and `--lang` as dictation.

## Narrate an accessibility tree

Renders a Chrome accessibility tree as the announcements a screen reader would
make, and speaks them. Feed it
[chrome-devtools MCP](https://github.com/ChromeDevTools/chrome-devtools-mcp)'s
`take_snapshot` output.

```bash
local-whisper a11y snapshot.txt
local-whisper a11y --mode headings --quiet snapshot.txt   # print, don't speak
pbpaste | local-whisper a11y --mode links
```

| Flag | Does |
|---|---|
| `--mode` | `reading` (default), `headings`, `links`, `landmarks`, `forms` |
| `--quiet` | Print the announcements without speaking them |
| `--voice`, `--speed` | As for `speak` |

It also reports what only shows up when a page is heard in order: unlabeled
controls, several links that announce identically, skipped heading levels.
Those findings always cover the whole page, even when `--mode` narrated part
of it. For rule-based violations, run chrome-devtools' `lighthouse_audit`.

## Engine server

```bash
local-whisper engine start    # background the mlx-engine STT/TTS server
local-whisper engine status
local-whisper engine stop
```

Run from the repo root, or pass `--script` — it drives
`scripts/mlx-engine-server.sh`, which owns the Python venv.

## MCP server

```bash
local-whisper mcp             # stdio; see docs/mcp.md
```

## Choosing an engine

`whisper` is the default and needs nothing beyond `brew install whisper-cpp`.
`voxtral` is opt-in (`make setup-voxtral`) and downloads its model on first
real use.

| | `whisper` (default) | `voxtral` |
|---|---|---|
| Setup | `brew install whisper-cpp` | `make setup-voxtral`, Apple Silicon only |
| How it runs | A subprocess per transcription, reloading the model each time | A persistent local server on `127.0.0.1:8765`, started on demand |
| Model | `ggml-base.en.bin` (141MB), or `--model tiny` (74MB) | `Voxtral-Mini-4B-Realtime-2602-4bit` |
| Accuracy | Fine for everyday dictation | Better, particularly on technical vocabulary |
| `.whisper-context` hints | Yes | No — see below |

The ~4% vs ~10% word-error-rate figures in
[`../mlx-engine/README.md`](../mlx-engine/README.md) compare Voxtral against
Whisper **Large-v3**, which is a much larger model than the `base.en` shipped
here. Treat them as a comparison between those two models, not as a measurement
of this default path.

**Known gap**: `.whisper-context` vocabulary hints work under `--engine
whisper` but aren't sent to the Voxtral server — the model can't take them. See
[`../mlx-engine/README.md`](../mlx-engine/README.md#known-limitation-whisper-context-doesnt-work-under---engine-voxtral).

## Context files

A `.whisper-context` file gives whisper.cpp vocabulary hints, which helps with
names, jargon and acronyms it would otherwise mangle.

```bash
# Global, applies everywhere
echo "Go, Golang, func, struct, goroutine, defer" > ~/.whisper-context

# Per project; takes precedence over the global one
echo "useEffect, useState, Redux, async, await" > .whisper-context
```

Override both with `--context path/to/file`.
