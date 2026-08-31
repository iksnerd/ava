# Claude Code voice hooks

[← Back to the README](../README.md)

Wires Claude Code's `Notification` and `Stop` hooks to speak through the
local `mlx-engine` server (`../mlx-engine/`) — you hear when Claude needs a
decision (a permission prompt, waiting on input) or has finished a turn,
without watching the terminal.

## Setup

Add to `~/.claude/settings.json` (global, not per-project — hooks fire for
every Claude Code session):

```json
{
  "hooks": {
    "Notification": [
      { "hooks": [{ "type": "command", "command": "bash /path/to/local-whisper/scripts/hook-notify.sh" }] }
    ],
    "Stop": [
      { "hooks": [{ "type": "command", "command": "bash /path/to/local-whisper/scripts/hook-stop.sh" }] }
    ]
  }
}
```

Both hooks prefix what they speak with the originating repo's name (resolved
from the hook payload's `cwd` via `scripts/repo-name.sh`), so overlapping
sessions across projects stay distinguishable — e.g. *"my-app: Claude
finished: ..."*. Both run fully backgrounded, so a slow TTS or LLM-summary
call never stalls the hook itself; Claude Code gets control back immediately
regardless of how long the actual speech takes.

## Scripts

| Script | Role |
|---|---|
| `lib.sh` | Shared: PATH hardening (hooks run with a minimal PATH that may not include `uv`/Homebrew dirs), hook JSON parsing, `config_get`/`config_get_int`/`config_get_bool`/`voice_is_muted`, `voice_hooks_run` (invokes the `voice_hooks/` CLIs below via `uv run`) |
| `speak.sh` | The actual "say this" entry point — starts the server on demand, calls `/speak`, plays via `afplay` with a cross-process lock so concurrent sessions queue instead of talking over each other, falls back to macOS `say` if the server's unreachable |
| `hook-notify.sh` | Speaks the Notification hook's `message` field verbatim (truncated per `notifyMaxChars`) |
| `hook-stop.sh` | Extracts Claude's last message from the transcript, then speaks it — see below for the length/summary logic |
| `voice_hooks/` | `uv`-managed Python package (flat scripts, no nested package — same pattern as `../voxtral/`): markdown stripping, `pysbd`-based sentence-boundary truncation, and Ollama summarization (`httpx`). `hook-stop.sh`/`hook-notify.sh` each shell out to it exactly once per firing via `lib.sh`'s `voice_hooks_run`. First-time setup: `make setup-voice-hooks`. |
| `repo-name.sh` | Resolves a `cwd` to a short repo/folder name |
| `voxtral-server.sh` | Start/stop/status for the `mlx-engine` server — atomic start-lock so concurrent sessions can't double-spawn it |

## Settings

Every setting below lives in `~/Library/Application Support/ClaudeVoice/config.json`
(edited live by `../ClaudeVoiceMenuBar/`) with `voice-defaults.json` as the
fallback for anything not yet in that file. An environment variable of the
matching name always overrides both, for one-off testing:

```bash
TTS_SPEED=1.0 ./scripts/speak.sh "test at normal speed"
```

| Config key | Env override | Default | Meaning |
|---|---|---|---|
| `muted` | — | `false` | Global kill switch — while on, `speak.sh` exits immediately (before starting the server, calling Ollama, or touching `afplay`) for every caller: both hooks, `Read Aloud`, and the menu bar app's Test/Preview buttons. Both hooks also check it themselves, before doing any transcript/truncation work, purely so a muted session doesn't pay for work whose result will never be heard. Toggle from the menu bar app, or system-wide via the `Toggle Claude Voice Mute` Service (see `../ClaudeVoiceMenuBar/README.md`). |
| `speed` | `TTS_SPEED` | 1.3 | Kokoro playback speed |
| `volume` | `TTS_VOLUME` | 1.0 | `afplay` volume |
| `voice` | — (2nd positional arg to `speak.sh`) | `af_heart` | Kokoro voice name |
| `sayRate` | `TTS_SAY_RATE` | 220 | `say` fallback words/min |
| `stopMaxChars` | `TTS_STOP_MAX_CHARS` | 600 | Length cap for Stop-hook messages |
| `notifyMaxChars` | `TTS_NOTIFY_MAX_CHARS` | 500 | Length cap for Notification messages |
| `llmSummary` | — | `false` | Summarize over-length Stop messages with Ollama instead of truncating |
| `summaryModel` | — | `qwen2.5:3b` | Ollama model used when `llmSummary` is on |

A length cap of `0` or less means **unlimited** (the menu bar app's "No
limit" toggle sets this).

## How `hook-stop.sh` decides what to say

`hook-stop.sh` resolves config (bash stays the single source of truth for
that) and passes it to `voice_hooks/stop.py`, which owns the actual decision:

1. If the message fits under `stopMaxChars` (or the cap is unlimited and the
   message is under a short internal sanity threshold), it's spoken
   **verbatim**.
2. Otherwise, if `llmSummary` is on, the message is sent to the local Ollama
   daemon (`http://127.0.0.1:11434`, via `httpx`) for a one-sentence summary
   sized to roughly `stopMaxChars` words.
3. If summarization is off, fails, or times out, it falls back to
   front-truncating at `stopMaxChars` while always keeping the message's true
   last sentence intact — real sentence segmentation (`pysbd`, not a naive
   `.!?` regex that mis-splits on abbreviations like "e.g.") decides where
   the lead-in is cut, but the final sentence (usually the actual conclusion)
   is never the part that gets dropped.
4. **Exception**: if the cap is unlimited, step 3's fallback is the full
   message, never a truncation — "No limit" is treated as a hard promise
   that a message is never cut off mid-sentence, even when summarization
   was attempted and didn't come back.

This all happens inside the `voice_hooks` CLI. If that call fails outright
instead of running and returning a result — unsynced venv, `uv` not on
`PATH` — both hooks fall back to speaking the raw, untruncated, unstripped
message rather than saying nothing. This fails *open*, not silent: run `make
setup-voice-hooks` once if you notice responses being read aloud in full or
with markdown noise instead of summarized or cleanly truncated.

## Requires

- `mlx-engine` running (auto-started on demand) — see `../mlx-engine/README.md`.
- `jq`, `curl`, `python3`, `uv` on `PATH`.
- `make setup-voice-hooks` run once (creates `scripts/voice_hooks/.venv`) —
  see the fallback note above for what happens if this is skipped.
- [Ollama](https://ollama.com) running locally, only if `llmSummary` is
  enabled (`ollama pull qwen2.5:3b` or whichever model `summaryModel` names).
