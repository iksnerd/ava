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

- **`ClaudeVoiceMenuBar` has no test target.** This is why the Swift half of
  the voice-config contract lives in `make check-swift-config` (a script that
  extracts `VoiceConfig` out of the source and runs it against
  `testdata/voice-config-cases.json`) instead of in `make test` — putting
  `swiftc` on the default test path would break `make test` on any machine
  without Xcode. It is also not a coincidence that the one config bug that
  actually shipped (a single wrong-typed value silently unmuting the app, fixed
  2026-09-21) lived in the one component nothing could test. Adding a target
  means splitting the package so the logic is importable, which is a real
  refactor, not a patch.

- **Run the `web-accessibility-audit` skill's trigger evals.** Five cases exist
  under `.claude/skills/web-accessibility-audit/evals/` (3 should-fire, 2
  near-miss) and have never been executed, so the skill is capped at
  `rubric v2: 89, Ready (unverified)`. `claude plugin eval` spawns agents and
  bills for them, hence deferred rather than run in passing.

- **Nothing has ever heard Kokoro on this machine.** `mlx-engine/server.py`
  pins `mlx-community/Kokoro-82M-bf16`, which has never been downloaded here
  (~330MB); the HF cache holds two unrelated models. Every spoken hook
  notification so far has come from the macOS `say` fallback. Starting the
  engine once and speaking a sentence is all it takes to close this, and it
  would also be the first real check of the `internal/speaker` playback path
  shipped on 2026-09-21, which so far has only been verified muted.

- **Decide whether `web-accessibility-audit` belongs in `iksnerd/skills`.** It
  lives in this repo's `.claude/skills/`, so it only fires while working here —
  not in the web projects where a page would actually be audited. Left local on
  purpose for now; promote with `code-quality:skill-distiller` if it earns it.

- **Nothing catches a shipped script or make target that no doc mentions.**
  `make setup-blackhole` existed, worked, and was referenced by nothing —
  `SETUP.md` walked a reader through installing BlackHole by hand without ever
  saying the repo automates the installable part. Found by listing every
  Makefile target and grepping the docs for it, which took one loop and should
  not need a person to think of running it. `scripts/check-portable-paths.sh`
  is the shape the fix would take: a check in `make lint` that fails when a
  target or a `scripts/*.sh` is mentioned in no `.md`, with an allowlist for
  the genuinely internal ones (`fmt-check`, `vet`).

## Done (2026-09-22, later)

- Audited the docs against the code rather than against memory, after the
  README split and the port/path fixes, and closed the three gaps it found:
  `make check-paths` was missing from the README's Development block though
  `check-swift-config` was listed; `SETUP.md` never mentioned
  `make setup-blackhole`; and `ClaudeVoiceMenuBar/README.md` still described
  the global mute as something `speak.sh` enforces "for every caller", which
  stopped being the whole story when the Go side gained its own readers of the
  same flag. Everything else checked clean: all seven CLI commands and all
  their flags appear in `docs/cli.md`, all five MCP tools in `docs/mcp.md`, and
  every remaining `8765` in the tree belongs to mlx-engine.

## Done (2026-09-22)

- Moved `cmd/voice-monitor` off port 8765 to 8766. It had defaulted to
  mlx-engine's port, which `CLAUDE.md`'s own Ports convention forbids — the
  convention exists because the engine was moved off 8000 for the same reason.
  Whichever server started second failed to bind, and the engine is up whenever
  anything has used `--engine voxtral` or spoken, so in practice it was
  `voice-monitor` that lost. Guarded by a test that reads the engine's port out
  of `pkg/mlx` rather than restating it, so the two cannot drift back together;
  `SETUP.md`, `setup-blackhole.sh` and the architecture diagram follow, and the
  convention now names both ports instead of only the one to avoid.

- Removed the two hardcoded home directories from tracked files: a
  `/Users/<name>/.local/bin` PATH entry in `scripts/lib.sh` (now `$HOME`) and a
  hardcoded repo root in `ClaudeVoiceMenuBar/Paths.swift` (now derived from its
  own `#filePath`, four levels up, verified with a probe at a known depth
  rather than assumed). This repo is public, and both were correct forever on
  one machine and broken from the first clone by anyone else — silently, since
  a PATH entry that does not exist is not an error. Escalated to
  `scripts/check-portable-paths.sh`, which runs inside `make lint` and was
  confirmed to fail on a planted violation.

## Done (2026-09-21)

- Gave the local voice stack a Go speech path and exposed it: `pkg/mlx.Speak`
  (where that package's own doc comment already said a `Speak` belonged),
  `internal/voiceconfig`, `internal/speaker` and `internal/a11y`, then five CLI
  verbs (`speak`, `stop`, `voices`, `transcribe`, `a11y`) and
  `local-whisper mcp` serving the same five capabilities over stdio. Before
  this, speech was reachable from the bash scripts and the menu bar app but not
  from the CLI and not at all from another program. `internal/speaker`
  deliberately joins `speak.sh`'s existing protocol rather than forking it —
  same mute gate, same activity markers, same `flock(2)` on the shared playback
  lock (Python's `fcntl.flock` is `flock(2)`, so they genuinely queue) — and
  writes no `.synth.pid`, because synthesis here is an in-process HTTP call and
  naming our own PID would have a stop kill the server. Adds
  `github.com/modelcontextprotocol/go-sdk` as the repo's second third-party Go
  dependency, on the same terms as the cobra exception.

- Fixed the three readers of `ClaudeVoice/config.json` disagreeing on malformed
  input, which had shipped. `VoiceSettings.swift` decoded through the
  synthesized `Codable` init, which is all-or-nothing: one hand-edited
  `"speed": "fast"` failed the whole decode and fell back to every property
  default, `muted = false` included — so the menu bar app would show unmuted
  and speak while the hooks and the CLI stayed correctly silent. `bash` had a
  separate hole in the same place, passing a non-numeric speed into
  `float()` and dropping synthesis to the `say` fallback with no error
  anywhere. Both now degrade per key. The bash hole was found by the new
  contract test on its first run, not by reading the code — an hour after that
  reader had been examined and written off as correct. Cases live in
  `testdata/voice-config-cases.json`; `internal/voiceconfig/contract_test.go`
  fails when bash and Go *disagree*, not merely when either is wrong alone, and
  both arms were mutation-checked before being trusted.

- Rewrote the README as a landing page (287 lines → 191) and split the
  reference into `docs/cli.md`, `docs/troubleshooting.md` and a validated
  `docs/architecture.mmd`. The package-layout block was deleted rather than
  moved, since `AGENTS.md` already maintains that map and the README's copy had
  drifted. Verification caught a claim that was wrong in the project's favour:
  the engine table credited the default whisper path with "~10% WER (Whisper
  Large-v3-class)", when the default model is `ggml-base.en` and those figures
  compare Voxtral against Large-v3. Also fixed a cold-start figure that
  contradicted itself between two sections, a `-engine voxtral` spelling left
  over from the Cobra migration, and a diagram node naming a symbol that no
  longer exists.

## Done (2026-09-01)

- Extracted `scripts/mlx-engine-server.sh`'s PID-file-alive check (the
  exact `[ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null`
  pattern was repeated identically in its `start`/`stop`/`status` cases —
  real, present duplication within one file, not just hypothetical future
  reuse) and its mkdir-based start lock into `scripts/lib.sh` as
  `server_pidfile_alive`/`server_lock_acquire`/`server_lock_release`,
  following the repo's existing "one shared lib every script sources"
  pattern rather than a new file. Verified the extraction safely against
  fake PID files/lockdirs (alive PID, dead PID, missing file, stale-lock
  reclaim) rather than against the real running mlx-engine server, since
  a live one was already up and likely backing real TTS use.
- Follow-up refinements to the `pkg/stt/` restructuring above, same day:
  - Moved `pkg/stt/mlx` → top-level `pkg/mlx`. `mlx-engine` (the server it
    wraps) serves TTS (Kokoro `/speak`) as much as STT (`/transcribe`), so
    nesting the Go client under `pkg/stt/` mis-scoped it as STT-only. It
    still satisfies `stt.Client` today (structurally, via `Transcribe()`),
    it just doesn't live inside that package anymore — if a real Go TTS
    caller ever shows up, a `Speak()` method belongs on this same client,
    not a new package.
  - Extracted the near-identical "write text to `opts.OutputPath` if set,
    warn-don't-error on failure" block that both `pkg/stt/whisper` and
    `pkg/mlx` had duplicated (same 5 lines, same comment) into
    `stt.WriteOutputIfRequested(opts, text)` in `pkg/stt/stt.go`. Both
    engines call it now instead of repeating the block.
  - Removed `voxtral-migration.md`. Read it in full first: most of it
    (the "why Voxtral" rationale, the Phase 1-5 changelog) was superseded
    by current docs and had no remaining operational value. The one
    genuinely still-relevant piece — the technical explanation of why
    `.whisper-context` doesn't work under `-engine voxtral` — was moved
    into `mlx-engine/README.md`'s Models section (a more natural home,
    verified the new heading's exact GitHub anchor with `github-slugger`
    rather than guess) before deleting the file, and `README.md`'s two
    live references (the Known gap link, the Documentation index entry)
    were updated/removed accordingly. `voxtral-migration.md`'s own Phase 3
    description of `pkg/voxtral/voxtral.go` as an HTTP client was accurate
    history either way, but the file as a whole wasn't worth keeping just
    for that.
- Restructured the Go STT engines under a single `pkg/stt/` hierarchy:
  `pkg/transcribe`→`pkg/stt` (the shared `Options`/`Client` contract),
  `pkg/whisper`→`pkg/stt/whisper`, `pkg/mlxengine`→`pkg/stt/mlx`,
  `pkg/realtimestt`→`pkg/stt/realtime` (that one was `pkg/voxtral` just
  two commits ago — this consolidates it further now that the domain
  grouping exists). No `pkg/tts` created: confirmed zero Go TTS/synthesis
  code exists (`internal/ttscontrol` only cancels in-flight speech, a Go
  port of `stop-speaking.sh`'s marker protocol — it never synthesizes
  anything; all real TTS goes through `scripts/speak.sh` curling
  `mlx-engine`'s `/speak`), so an empty package would've been speculative
  structure — documented instead, in `pkg/stt`'s own doc comment.
- Renamed `scripts/voxtral-server.sh` → `scripts/mlx-engine-server.sh` —
  another instance of the same bug class as the `pkg/voxtral` rename: the
  script never touched `voxtral/` at all, it manages **`mlx-engine`**
  (`cd`s into `mlx-engine/`; its own PID/log files were literally
  `voxtral-server.pid`/`.log`). Updated every referrer: `Makefile`
  (`start-engine`/`stop-engine`/`status-engine` targets, and the
  generated Raycast `voxtral-toggle.sh`'s PID-file check — that generated
  script's own name/title stay put, they're about the `-engine voxtral`
  choice, not this server script), `scripts/speak.sh`,
  `cmd/local-whisper/main.go`'s error strings, `ServerController.swift`,
  and doc mentions (`README.md` — including a log-path reference in
  Troubleshooting that would've silently gone stale — `AGENTS.md`,
  `mlx-engine/README.md`, `ClaudeVoiceMenuBar/README.md`,
  `docs/claude-code-voice-hooks.md`).
- Left `voxtral/` (the Python directory), `--voxtral-dir`,
  `make setup-voxtral`, and `scripts/setup-voxtral.sh` alone, deliberately
  — decided the basic/advanced framing below makes "voxtral" a *correct*
  label for these (the thing you opt into for the advanced tier), unlike
  the server script, which was actively wrong. Renaming these would've
  been pure churn fixing nothing.
- Made the "basic vs. advanced" engine framing explicit in `CLAUDE.md`,
  `AGENTS.md`, and `README.md`'s Voice Engines section: `whisper.cpp` +
  Kokoro are the default, zero-setup path; Voxtral models are the
  opt-in advanced tier (`make setup-voxtral`, lazy-loaded on first real
  use). This was already true mechanically before today — this pass
  just says so in the docs instead of leaving it implicit.
- Removed `voxtral/stt.py` too — same dead-code profile as `tts.py` below:
  confirmed zero callers anywhere (not wrapped in Go since the earlier
  `Client.Transcribe` removal, no shell script invokes it, no doc gave it
  as a real workflow). Its underlying one-shot Voxtral Mini 3B model was a
  road not taken for `local-whisper -engine voxtral`, which went with
  `pkg/mlxengine`'s HTTP-server architecture instead (see
  `voxtral-migration.md`) — same reasoning as `tts.py`.
- Renamed `pkg/voxtral` → `pkg/realtimestt`. Its only remaining surface
  after the `Transcribe`/`Speak` and now `stt.py` removals is
  `StreamRealtime`/`ListInputDevices` — a subprocess wrapper around
  `voxtral/realtime.py`, used only by `cmd/voice-monitor`. The old name
  was actively causing bugs: `AGENTS.md` had it backwards in ~4 places,
  describing `pkg/voxtral` as "the mlx-engine HTTP client" — that's
  actually `pkg/mlxengine`'s job (confirmed via `pkg/mlxengine.Client`'s
  actual methods). Rename was well-contained: only one Go file imports
  each of `pkg/realtimestt` (`cmd/voice-monitor/main.go`) and
  `pkg/mlxengine` (`cmd/local-whisper/main.go`). `voxtral-migration.md`'s
  own "Phase 3" line describing `pkg/voxtral/voxtral.go` as "HTTP client
  posting audio to /transcribe" was **not** touched — that's accurate
  history (that package really was the HTTP client at that phase, before
  it was renamed to `pkg/mlxengine` and `voxtral` got reused for
  `cmd/voice-monitor`'s later, differently-purposed package) — not a bug
  to retroactively fix in a design record.
- Removed `voxtral/tts.py` too (follow-up to the `Client.Transcribe`/`Speak`
  removal below, which had left it as "untouched, still independently
  runnable"). Confirmed Kokoro (via `mlx-engine`) is the only TTS engine
  actually in the live pipeline — every speaking path (hooks, Read Aloud,
  Mute Service confirmation, Test/Preview) goes through `speak.sh` →
  `mlx-engine`'s `/speak`, falling back to macOS `say`, never anything in
  `voxtral/`. `tts.py`'s only reachability was the now-removed Go wrapper
  and a manual smoke-test line in `setup-voxtral.sh` (updated to demo
  `realtime.py --list-devices` instead). `stt.py` is unaffected and still
  there, still manually runnable, still not wrapped in Go (only
  `realtime.py` is, via `StreamRealtime`/`ListInputDevices`).

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
