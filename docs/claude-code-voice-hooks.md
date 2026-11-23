# Claude Code voice hooks

[← Back to the README](../README.md)

Wires Claude Code's `Notification` and `Stop` hooks to speak through the local
`mlx-engine` server, so you hear when Claude needs a decision or has finished a
turn without watching the terminal.

## Setup

The hooks live in this repo's `scripts/` directory, so they need a checkout.
They speak through `ava speak`: build it with `make build` (the hooks find the
checkout's `bin/ava`) or install a release (`ava` on PATH or in `~/.local/bin`).
Without one, `speak.sh` exits non-zero and says so. Run `make setup-voice-hooks`
once to create the hooks' Python environment.

Add to `~/.claude/settings.json` (global, not per-project, so the hooks fire in
every Claude Code session). Replace `/path/to/ava` with your checkout:

```json
{
  "hooks": {
    "Notification": [
      { "hooks": [{ "type": "command", "command": "bash /path/to/ava/scripts/hook-notify.sh" }] }
    ],
    "Stop": [
      { "hooks": [{ "type": "command", "command": "bash /path/to/ava/scripts/hook-stop.sh" }] }
    ]
  }
}
```

Both hooks prefix what they speak with the repo's name (from the hook payload's
`cwd`), such as "my-app: Claude finished: ...", so overlapping sessions stay
distinguishable. They run in the background, so slow speech or summarization
never stalls Claude Code.

The scripts in `scripts/` are thin entry points: `speak.sh` checks the mute and
hands off to `ava speak --async`, and `hook-stop.sh` and `hook-notify.sh` do
their text processing in `scripts/voice_hooks/`. `repo-name.sh` resolves the repo
name, and `mlx-engine-server.sh` starts and stops the engine.

## Settings

Every setting below lives in `~/Library/Application Support/ava/config.json`
(edited live by the menu bar app) with `voice-defaults.json` as the
fallback for anything not yet in that file. An environment variable of the
matching name always overrides both, for one-off testing:

```bash
TTS_SPEED=1.0 ./scripts/speak.sh "test at normal speed"
```

| Config key | Env override | Default | Meaning |
|---|---|---|---|
| `muted` | none | `false` | Global kill switch, described below the table |
| `speed` | `TTS_SPEED` | 1.3 | Kokoro playback speed |
| `volume` | `TTS_VOLUME` | 1.0 | `afplay` volume |
| `voice` | none (2nd argument to `speak.sh`) | `af_heart` | Kokoro voice name |
| `sayRate` | `TTS_SAY_RATE` | 220 | `say` fallback words/min |
| `stopMaxChars` | `TTS_STOP_MAX_CHARS` | 600 | Length cap for Stop-hook messages |
| `notifyMaxChars` | `TTS_NOTIFY_MAX_CHARS` | 500 | Length cap for Notification messages |
| `llmSummary` | none | `false` | Summarize over-length Stop messages with Ollama instead of truncating |
| `summaryModel` | none | `qwen2.5:3b` | Ollama model used when `llmSummary` is on. If it is not installed, summarization falls back to truncation |
| `engineAutoStart` | none | `true` | Whether a hook may start `mlx-engine` on demand. `ava engine stop` and the menu bar Stop button set it to `false`; `engine start` sets it back. While off, speech falls back to macOS `say` |

A length cap of `0` or less means unlimited (the menu bar app's "No limit"
toggle sets it).

**Mute.** While `muted` is on, `speak.sh` exits before starting the server,
calling Ollama or playing anything, for every caller: both hooks, `Read Aloud`
and the menu bar app's Test/Preview buttons. `ava speak`, `a11y` and the MCP
`speak` tool honour it too and say so rather than reporting success. Toggle it
from the menu bar app, or system-wide with the `Toggle Ava Mute` Service (see
`../AvaMenuBar/README.md`).

## How `hook-stop.sh` decides what to say

- A message under `stopMaxChars` is spoken verbatim.
- A longer one is summarized by the local Ollama daemon
  (`http://127.0.0.1:11434`) when `llmSummary` is on, to roughly the cap's
  spoken length.
- If summarization is off, fails or times out, the message is truncated at
  `stopMaxChars`, always keeping its last sentence.
- With the cap unlimited, the full message is spoken and never truncated.

If the `voice_hooks` call itself fails (venv not synced, `uv` not on `PATH`),
both hooks speak the raw message instead of staying silent. If you hear whole
responses or markdown noise, run `make setup-voice-hooks`.

## Requires

- `mlx-engine` running (auto-started on demand): see `../mlx-engine/README.md`.
- `jq`, `curl`, `python3`, `uv` on `PATH`.
- `make setup-voice-hooks` run once (creates `scripts/voice_hooks/.venv`).
- [Ollama](https://ollama.com) running locally, only if `llmSummary` is
  enabled (`ollama pull qwen2.5:3b` or whichever model `summaryModel` names).
