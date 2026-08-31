# TODO

Tracked here because they're deliberately deferred, not because they're
urgent — see CLAUDE.md for the project overview.

## Open

- **Shippable `.dmg` for `ClaudeVoiceMenuBar`** (currently only installable by
  building from this exact checkout — see `docs/claude-code-voice-hooks.md`
  and the app's own README). Today the app hard-fails on any other Mac
  because `Paths.swift:6-7` compiles in this literal absolute checkout path
  (`/Users/user/GolandProjects/local-whisper`) with no bundle-relative
  resolution, and `build-app.sh` only ad-hoc-signs (`codesign -s -`, no
  Developer ID, not notarized) and installs straight to `/Applications` —
  there's no `hdiutil`/`create-dmg` step at all. A real path to a
  distributable `.dmg` needs, roughly:
  - `Paths.swift` resolving `scriptsDir`/`repoRoot` relative to
    `Bundle.main` (or a first-run "where's your local-whisper checkout"
    prompt) instead of a compiled-in literal.
  - Either bundling `scripts/`, `mlx-engine/`, and a built `local-whisper`
    binary into the `.app`'s `Resources/`, or a first-run installer step
    that clones/sets them up (plus `sox`/`whisper-cli`/`uv`/Go via Homebrew
    if missing).
  - Deciding the Voxtral/MLX story for non-Apple-Silicon recipients — the
    default `whisper` engine has no such requirement, so a bundled build
    could ship whisper-only and treat `-engine voxtral` (and its `uv`/MLX/
    multi-GB model download) as an optional, Apple-Silicon-gated add-on
    rather than a hard dependency of the app itself.
  - **Ollama should stay optional, not bundled** — `scripts/voice_hooks/`'s
    `ollama_summarize` already degrades gracefully when it's unreachable
    (`ollama_client.py`, covered by `test_ollama_summarize_connection_refused`),
    so the installer/DMG just needs to leave `llmSummary` off by default and
    document Ollama as a manual opt-in, not try to ship/install it.
  - Real Developer ID signing + notarization (`codesign --sign
    "Developer ID Application: ..."`, `xcrun notarytool`) instead of
    ad-hoc, or Gatekeeper blocks it on first launch for any recipient.

## Done (2026-09-01)

- **Investigated, no change made**: `checkDependencies`'s voxtral health
  check (`cmd/local-whisper/main.go`) — the TODO's premise (a TCP dial
  before the HTTP GET would shorten the "server down" case) doesn't hold.
  Measured directly: a `client.Get` against a closed local port returns
  `connection refused` in ~1ms, not anywhere near the 1s timeout — a
  refused TCP connection is already near-instant on this OS, so a
  preliminary dial saves nothing. The only way to actually burn the full
  1s is a process that's listening but never responds (still loading, a
  hung handler) — and a TCP dial can't distinguish that from "healthy"
  either, since `accept()` succeeds instantly either way; you still need
  the HTTP-level timeout to catch it. No proxy env vars were set to test
  that alternate theory, but even under a misconfigured `HTTP_PROXY`,
  `client.Timeout` still bounds the whole round trip at 1s regardless, so
  it can't be a way to exceed the current behavior. Leaving the code as-is.
- Removed `pkg/voxtral.Client.Transcribe`/`Speak` (and `TranscribeOptions`/
  `SpeakOptions`) — decided against keeping them as speculative public API.
  Nothing in the repo called them: `cmd/voice-monitor` only ever used
  `StreamRealtime`/`ListInputDevices`, and `cmd/local-whisper`'s
  `-engine voxtral` goes through `pkg/mlxengine` instead. The underlying
  `stt.py`/`tts.py` scripts are untouched and still independently runnable
  (see `SETUP.md`'s manual smoke-test note) — only the unused Go wrapper and
  its 8 tests are gone. Recreating it later, if a one-shot voxtral CLI is
  ever actually built, is a trivial ~50-line addition following the same
  pattern already established twice (`pkg/whisper`, `pkg/mlxengine`).
- Bounded `cmd/voice-monitor`'s `hub.history` at 1MiB (`maxHistoryBytes`) so
  it no longer grows unbounded for the lifetime of a session — trims from
  the front on overflow, advancing to the next UTF-8 rune boundary so a
  trimmed snapshot never starts mid-character (transcripts can be non-ASCII:
  Bulgarian via the whisper engine, diarization labels, etc.).

## Done (2026-08-30)

- Merged audio normalization into recording: `internal/recording.Record()`
  now records and peak-normalizes (`norm -3`) in a single sox invocation,
  removing a full sox subprocess spawn + intermediate temp file from every
  dictation. `internal/audio` is now just the shared sox format constants.
- Added `pkg/transcribe` (`Options` + `Client` interface) so
  `cmd/local-whisper` selects between `pkg/whisper` and `pkg/mlxengine` via
  one interface instead of two near-duplicate branches.
- Fixed `internal/procutil.Silence` opening `/dev/null` read-only instead of
  write-only (EBADF risk for any subprocess that checks its write results).
- Fixed an HTTP response body leak in `checkDependencies` on non-200.
- Fixed a race in `pkg/voxtral.StreamRealtime` where `stop()` could return
  before the last buffered realtime deltas were scanned.
- Added fixture-driven test coverage across the board (`testdata/bin/`
  fake executables + `httptest`), including `pkg/mlxengine` and
  `cmd/voice-monitor`, which previously had none.
- `pkg/whisper.Client.Transcribe` now captures whisper-cli's stdout directly
  (`-nt`/`-sns`, no `-otxt`/`-of`) instead of round-tripping through a
  `.txt` file, removing a disk write+read and the `procutil.Silence`
  dependency per transcription. `OutputPath` is now written by us as a
  secondary, non-fatal step (parity with `pkg/mlxengine`). Bonus: failures
  now surface whisper-cli's actual stderr instead of a bare "exit status N"
  — verified live against a real model (missing-audio-file case: now
  reports `error: input file not found '...'` instead of nothing).
