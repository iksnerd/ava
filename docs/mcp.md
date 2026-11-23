# MCP server

[← Back to the README](../README.md)

`ava mcp` serves Ava's on-device Kokoro TTS and whisper.cpp STT to any MCP
client over stdio: Claude Code in any repo, Claude Desktop, or another agent.

```bash
claude mcp add -s user ava -- "$HOME/.local/bin/ava" mcp
```

Restart the session afterwards, since MCP servers load at session start. Use an
absolute path to `ava`: the client's working directory is not this repo.

`speak` starts the Kokoro server on demand. If the engine came from a
checkout (`make setup`) rather than `ava setup`, add
`-e MLX_ENGINE_SCRIPT="/path/to/ava/scripts/mlx-engine-server.sh"` to the
command. With no engine to start, `speak` falls back to the macOS `say` voice
and says so on stderr.

Pass `--server-url` if `mlx-engine` is not on `http://127.0.0.1:8765`.

## Tools

| Tool | Arguments | What it does |
|---|---|---|
| `speak` | `text`, `voice?`, `speed?`, `async?`, `out?`, `timings?`, `split?`, `out_dir?` | Speaks through Kokoro, falling back to macOS `say` when the server is down. With `out` it saves a WAV to that path instead, with no mute and no `say` fallback; `timings` also writes each segment's start and end as JSON, cut by `split` (`line` or `sentence`). With `out_dir` each non-blank line becomes its own WAV in that directory, with a `manifest.json` |
| `stop_speaking` | none | Cancels speech in flight, including another session's hook speech |
| `list_voices` | none | The available Kokoro voices and their accent/gender |
| `transcribe` | `audio_path`, `language?`, `model?`, `beam_size?`, `format?` | Transcribes a 16kHz mono WAV on-device; `format` is `text`, or `srt`, `vtt` or `json` for timed captions |
| `speak_accessibility_tree` | `snapshot`, `mode?`, `speak?`, `voice?`, `speed?` | Renders a Chrome accessibility tree as screen-reader announcements and speaks them |

MCP speech behaves like hook speech. The menu bar app's global Mute silences it
(the tool says so), its speaking indicator lights up, `stop_speaking` and the
menu bar Stop both cancel it, and concurrent speaks queue instead of
overlapping.

## Pairing with chrome-devtools MCP: hearing a web page

`speak_accessibility_tree` takes [chrome-devtools
MCP](https://github.com/ChromeDevTools/chrome-devtools-mcp)'s `take_snapshot`
output (Chrome's real accessibility tree) and renders it the way a screen
reader would announce it:

```
uid=1_2 link "Home" focusable      ->  link, Home
uid=1_5 heading "Overview" level="1"  ->  heading level 1, Overview
uid=1_12 button ""                 ->  button, unlabeled
```

Modes: `reading` (whole page in order), `headings`, `links`, `landmarks`,
`forms`. `speak: false` makes it a pure formatter for a quiet pass.

It also reports what only shows up when a page is heard in order: unlabeled
controls, links that announce identically, skipped heading levels and a missing
level-1 heading. Rule-based violations are `lighthouse_audit`'s job.

The full workflow lives in
[`.claude/skills/web-accessibility-audit/`](../.claude/skills/web-accessibility-audit/SKILL.md).

## Smoke-testing it

Keep the pipe open until the replies arrive, or stdin hits EOF and the server
exits without answering. This sends `initialize` and `tools/list` and prints two
JSON lines; the second lists the five tools above:

```bash
(printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'
 sleep 2) | ava mcp 2>/dev/null
```
