# TODO

Tracked here because they're deliberately deferred, not because they're
urgent — see CLAUDE.md for the project overview.

## Open

- **Real Developer ID signing + notarization for `ClaudeVoiceMenuBar`**
  (`codesign --sign "Developer ID Application: ..."`, `xcrun notarytool`)
  — no Apple Developer Program membership available to do this now. Until
  then, `.build/Claude Voice.dmg` stays ad-hoc signed (`codesign -s -`).
  Verified directly (mounted the `.dmg`, applied a real quarantine
  attribute, tried to launch it): a recipient hits Gatekeeper's standard
  "Apple could not verify ... is free of malware" warning, not the harsher
  "is damaged" message — fixable via System Settings → Privacy & Security
  → "Open Anyway", or `xattr -cr "/Applications/Claude Voice.app"` (also
  verified: launches clean afterward, no further prompt). Fine for sharing
  with yourself/friends; not for wide public distribution.

## Done (2026-09-01)

- **Shippable `.dmg` for `ClaudeVoiceMenuBar`**, stage 1 (bundling +
  packaging; real signing/notarization above is what's left):
  - `Paths.swift` no longer hard-fails on another Mac — it resolves
    `scriptsDir`/`dictateBinary` from the app's own bundled `Resources/`
    when running as a packaged `.app`, falling back to this dev checkout's
    literal path only when running unbundled (`swift run`). Verified this
    isn't a silent no-op: added temporary debug instrumentation, rebuilt,
    launched the real installed `.app`, and confirmed both paths resolved
    to `/Applications/Claude Voice.app/Contents/Resources/...`, not the
    dev checkout — then reverted the instrumentation.
  - `build-app.sh` now also builds `local-whisper` and bundles it +
    `scripts/` (excluding `.venv`/`__pycache__`) into `Resources/`, then
    produces `.build/Claude Voice.dmg` via `hdiutil`.
  - Decided against bundling `mlx-engine/` — its venv alone is 1.3GB, plus
    a 2.9GB Voxtral model download. Confirmed the only thing that actually
    depends on its location is `voxtral-server.sh` (the Server section's
    Start/Stop button); `speak.sh`/hooks/`voice_hooks/` don't touch it and
    `speak.sh` already falls back to macOS `say` when the server's
    unreachable, so the packaged app is fully functional without it.
  - Dictate (`SettingsView.swift`) no longer hardcodes `-engine voxtral` —
    it now uses `local-whisper`'s own default (`whisper`, 141MB model,
    any Mac), so a fresh packaged install's Dictate button works with zero
    extra setup instead of requiring Apple Silicon + a separate
    `make setup-voxtral`.
  - Ollama was already optional/opt-in by default (`llmSummary: false`) —
    no change needed there.

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
