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

## Installing on another machine

```bash
bash scripts/install.sh          # latest release; or pass a tag
local-whisper setup
```

`install.sh` fetches the release with `gh` (which carries your credentials, so
it works while the repo is private) or `curl`, verifies the checksum before
extracting, and installs both binaries to `~/.local/bin` —
`LOCAL_WHISPER_BIN` overrides that.

**Use it rather than downloading the archive in a browser.** The binaries are
unsigned: signing for distribution needs a Developer ID certificate and
notarization, which require a paid Apple Developer Program membership, and an
Xcode "Apple Development" certificate is not a substitute — it signs cleanly
and `spctl -a -t exec` still rejects the result.

That only matters for a browser download, because macOS attaches
`com.apple.quarantine` when a browser fetches a file and not when `curl` or
`gh` does. Two things are worth knowing about the case where it does happen,
both measured on macOS 27:

- **Extracting in a terminal does not launder it.** Quarantine survives
  `tar xzf`, whatever older advice says.
- **There is nothing to search for.** A quarantined unsigned binary run from a
  shell is killed with exit 137 and prints nothing at all, and the GUI dialog
  macOS shows offers **Move to Trash**, not Open. Getting past it means
  System Settings → Privacy & Security → "Open Anyway", or simply:

  ```bash
  xattr -d com.apple.quarantine local-whisper voice-monitor
  ```

## Setup

```bash
local-whisper setup                 # deps + model + Kokoro engine
local-whisper setup --skip-engine   # dictation only, without the 1.2 GB
```

Installs everything that is not the binary: `sox` and `whisper-cli` via
Homebrew, the whisper.cpp `base.en` model (~141 MB), and the Kokoro TTS engine.
Each step is skipped when it is already done, so re-running is cheap — and
re-running is how you refresh the engine after installing a newer binary.

It exists because the binary is otherwise only half usable without a checkout.
`make setup` does the same three things, but only from the repo, which is the
one thing a downloaded binary does not have. The engine is carried inside the
binary as ~600 KB of scripts, `server.py` and a `uv.lock`; the 1.2 GB of Python
is resolved by `uv` at install time and the 339 MB Kokoro model is fetched by
the server on first use, so neither is shipped.

It lands in `~/Library/Application Support/ava/engine/`, beside the config the
CLI and the menu bar app already share. `LOCAL_WHISPER_ENGINE_DIR` overrides
that, mainly so the install can be exercised against a throwaway directory.

**The install path has a length budget.** espeak-ng, which Kokoro phonemizes
through, keeps its data path in a 160-byte buffer and truncates past it — the
server then starts, loads the model and dies on a missing `phontab` against a
path that names neither the length nor the directory. `setup` measures this up
front and refuses rather than letting you find out after `uv` has resolved a
gigabyte. The default uses 128 of the 160 bytes.

Apple Silicon only for the engine step; on Intel it is skipped with a note, and
speech uses the macOS `say` voice.

### Just the model

```bash
local-whisper setup-model               # base.en (~141 MB)
local-whisper setup-model --model tiny  # tiny.en (~74 MB), for --model tiny
```

Downloads one whisper.cpp model to `~/.local/share/whisper-cpp/` and nothing
else; `setup` only fetches `base`. The download comes from a pinned Hugging
Face revision and is checked against its sha256 before it is installed. An
existing file of the right size is left alone, and one of the wrong size (an
interrupted download) is fetched again.

## Engine server

```bash
local-whisper engine start    # background the mlx-engine Kokoro TTS server
local-whisper engine status
local-whisper engine stop
local-whisper engine stop --keep-autostart   # temporary stop; hooks stay armed
```

Run from the repo root, or pass `--script` — it drives
`scripts/mlx-engine-server.sh`, which owns the Python venv. `make start-engine`,
`make stop-engine` and `make status-engine` are thin wrappers around the same
three commands.

`stop` also disarms on-demand auto-start (`engineAutoStart` in the config), so
the server stays stopped instead of the next hook bringing it back up; `start`
re-arms it, and `stop` now says which it did. See
[`claude-code-voice-hooks.md`](claude-code-voice-hooks.md#settings).

Use `stop --keep-autostart` when the stop is temporary — you started the engine
to check on it and want the machine back as you found it. Without it, undoing a
throwaway stop means starting the server again, which is the opposite of
tidying up, and the setting it changed outlives the command: every later hook
speaks through macOS `say` until something re-arms it.

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

Voxtral is still used, in `voice-monitor`: that path streams, where the wrapper
here transcribes a complete file per subprocess. (whisper.cpp does ship a
streaming example; it is not what `pkg/stt/whisper` wraps.) See
[`voice-monitor.md`](voice-monitor.md).

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
