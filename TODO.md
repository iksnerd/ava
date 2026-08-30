# TODO

Tracked here because they're deliberately deferred, not because they're
urgent — see CLAUDE.md for the project overview.

## Open

- **`pkg/voxtral.Client.Transcribe`/`Speak`** (the one-shot `stt.py`/`tts.py`
  wrappers) aren't called by any command today — `cmd/voice-monitor` only
  uses `StreamRealtime` and `ListInputDevices`, and `cmd/local-whisper`'s
  `-engine voxtral` goes through `pkg/mlxengine` instead. Decide whether
  they're kept as public API for a future one-shot voxtral CLI path, or
  removed if nothing will call them.
- **`cmd/voice-monitor`'s `hub.history`** grows unbounded for the lifetime of
  a session (`h.history = append(h.history, text...)` in `main.go`). Harmless
  at call-transcript text volumes, but there's no cap — worth a bound if
  voice-monitor is ever left running for very long unattended sessions.
- **`checkDependencies`'s voxtral health check** (`cmd/local-whisper/main.go`)
  has a 1s HTTP timeout, so every `-engine voxtral` run pays up to 1s of
  startup latency when the server happens to be down. Fine today; a cheap
  TCP dial before the HTTP GET would shorten the common "server not running
  at all" case if it ever matters.

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
