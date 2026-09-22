# MCP server

`ava mcp` serves the local voice stack to any MCP client over
stdio. Same on-device Kokoro TTS and whisper.cpp STT the CLI, the hooks
and the menu bar app use — but reachable from Claude Code in any repo,
Claude Desktop, or another agent, without this repo's `scripts/` directory
being on disk.

```bash
claude mcp add -s user ava \
  -e MLX_ENGINE_SCRIPT="$PWD/scripts/mlx-engine-server.sh" \
  -- "$HOME/.local/bin/ava" mcp
```

MCP servers load when a session starts, so restart the session afterwards.

Both details in that command matter for an installed binary. An absolute path
to it, because the MCP client's working directory is not this repo. And
`MLX_ENGINE_SCRIPT`, because `speak` starts the Kokoro server on demand, and
the MCP server's working directory is not this repo. Auto-start tries
`MLX_ENGINE_SCRIPT`, then `scripts/` under the working directory, then the
engine `ava setup` installed. So after `ava setup` you can drop the `-e` line;
with the engine from a checkout, which is what `make setup` gives you, you need
it. Without a script to run, `speak` still works but falls back to the macOS
`say` voice, and says so on stderr.

Pass `--server-url` if `mlx-engine` isn't on the default
`http://127.0.0.1:8765`.

## Tools

| Tool | Arguments | What it does |
|---|---|---|
| `speak` | `text`, `voice?`, `speed?`, `async?` | Speaks through Kokoro, falling back to macOS `say` when the server is down |
| `stop_speaking` | — | Cancels speech in flight, including another session's hook speech |
| `list_voices` | — | The available Kokoro voices and their accent/gender |
| `transcribe` | `audio_path`, `language?`, `model?`, `beam_size?` | Transcribes a 16kHz mono WAV on-device |
| `speak_accessibility_tree` | `snapshot`, `mode?`, `speak?`, `voice?`, `speed?` | Renders a Chrome accessibility tree as screen-reader announcements and speaks them |

Every tool goes through the same protocol `scripts/speak.sh` established, so
MCP speech behaves like hook speech: the menu bar app's global Mute silences
it (the tool says so rather than claiming success), its speaking indicator
lights up for it, `stop_speaking` and the menu bar Stop both cancel it, and
concurrent speaks queue on the shared playback lock instead of overlapping.

## Pairing with chrome-devtools MCP: hearing a web page

`speak_accessibility_tree` takes [chrome-devtools
MCP](https://github.com/ChromeDevTools/chrome-devtools-mcp)'s `take_snapshot`
output — Chrome's real accessibility tree — and renders it the way a screen
reader would announce it:

```
uid=1_2 link "Home" focusable      ->  link, Home
uid=1_5 heading "Overview" level="1"  ->  heading level 1, Overview
uid=1_12 button ""                 ->  button, unlabeled
```

Modes: `reading` (whole page in order), `headings`, `links`, `landmarks`,
`forms`. `speak: false` makes it a pure formatter for a quiet pass.

Alongside the announcements it reports what only shows up once a page is
heard in order — unlabeled controls, several links that announce
identically, skipped heading levels, a missing level-1 heading. Rule-based
violations are `lighthouse_audit`'s job; these are the ones no rule catches.

The full workflow lives in
[`.claude/skills/web-accessibility-audit/`](../.claude/skills/web-accessibility-audit/SKILL.md).

## Smoke-testing it

Piping requests in with `echo` doesn't work: stdin hits EOF before the
handshake completes and the server exits without replying. Drive it with a
client that keeps the pipe open — the repo's own tests do this over the
SDK's in-memory transport:

```bash
go test ./cmd/ava/ -run 'Mcp|Tool|SpeakAccessibilityTree' -v
```

That pattern selects the tests in `mcp_test.go`; they are named after the
tool they exercise rather than after MCP.

## Implementation notes

- The server is `cmd/ava/mcp.go`, built on
  [`github.com/modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk).
- Nothing may write to stdout while it runs: that's the JSON-RPC channel.
  Diagnostics go to stderr.
- Speech goes through `internal/speaker`, settings through
  `internal/voiceconfig`, narration through `internal/a11y`.
