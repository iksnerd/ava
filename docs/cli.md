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
| `--model` | `base` | `base` (141MB) or `tiny` (74MB, faster, less accurate) |
| `--lang` | `en` | Language code: `en`, `es`, `fr`, `de`, … |
| `--context` | — | Path to a context file (see below) |
| `--output` | — | Also write the transcript to this file |
| `--dir` | — | Change to this directory first |
| `--no-paste` | off | Copy to the clipboard but don't paste |
| `--no-sound` | off | No audio cues |
| `--verbose` | `true` | Status messages; `--quiet` is the inverse |

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
local-whisper transcribe --lang es clip.wav
local-whisper transcribe --output notes.txt meeting.wav
```

Takes a WAV file. Same `--model` and `--lang` as dictation.

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
local-whisper engine start    # background the mlx-engine Kokoro TTS server
local-whisper engine status
local-whisper engine stop
```

Run from the repo root, or pass `--script` — it drives
`scripts/mlx-engine-server.sh`, which owns the Python venv.

`stop` also disarms on-demand auto-start (`engineAutoStart` in the config), so
the server stays stopped instead of the next hook bringing it back up; `start`
re-arms it. See
[`claude-code-voice-hooks.md`](claude-code-voice-hooks.md#settings).

`--script` only covers the explicit `engine` commands. The **implicit**
auto-start — the one a hook or `local-whisper speak` triggers when the server is
down — resolves the same script relative to the working directory, which is
wrong for a binary installed to `~/.local/bin`. Set `MLX_ENGINE_SCRIPT` to an
absolute path for that case:

```bash
export MLX_ENGINE_SCRIPT="$HOME/src/local-whisper/scripts/mlx-engine-server.sh"
```

Without it, an installed binary run from anywhere else falls back to macOS `say`
rather than starting Kokoro — and now says so on stderr when it does.

## MCP server

```bash
local-whisper mcp             # stdio; see docs/mcp.md
```

## Why there is only one transcription engine

`local-whisper` transcribes with whisper.cpp and nothing else. There used to be
a `--engine voxtral` that went through mlx-engine's HTTP server; it was removed
after measuring both on the same 20-second sample:

| | whisper.cpp | Voxtral 4B via mlx-engine |
|---|---|---|
| Warm | **1.26s** | 17.0s |
| Cold | same — no server | 126.7s, including a 108s model load |
| Model on disk | 141MB | 2.9GB |
| Peak memory | none held | ~4GB resident, 11GB while loading |
| Transcript | near-identical | near-identical |

The word-error-rate figures in
[`../mlx-engine/README.md`](../mlx-engine/README.md) compare Voxtral against
Whisper **Large-v3**, a far bigger model than the `base.en` here — they were
never a measurement of this path.

Voxtral is still used, in `voice-monitor`: that path streams at under 500ms,
which whisper.cpp cannot do at all, since it runs one subprocess per complete
file. See [`voice-monitor.md`](voice-monitor.md).

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
