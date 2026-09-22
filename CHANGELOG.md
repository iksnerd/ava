# Changelog

Notable changes. Dates are release dates; `v0.1.0` predates this file, so its
entry is a summary rather than a record kept as it happened.

## Unreleased

### Added
- **`local-whisper setup`** installs everything that is not the binary: `sox`
  and `whisper-cli` via Homebrew, the whisper.cpp `base.en` model, and the
  Kokoro TTS engine. `--skip-engine` leaves out the 1.2 GB for a
  dictation-only install. Until now the binary was only half usable without a
  checkout — `speak` fell back to the macOS `say` voice because the Python
  engine lives in the repo — and `make setup` only works from a repo, which is
  the one thing a downloaded binary does not have.
- The mlx-engine bundle now travels **inside** the binary (`internal/enginedist`,
  ~600 KB of scripts, `server.py` and a `uv.lock`). The 1.2 GB venv is resolved
  by `uv` at install time and the 339 MB voice model is fetched on first use, so
  neither ships. `engine start` finds the installed bundle when there is no
  repo, so the engine commands work from a bare binary.
- `setup` refuses an install path too long for espeak-ng's 160-byte data-path
  buffer. Past it the server starts, loads Kokoro, and dies on a missing
  `phontab` against a path that names neither the length nor the directory.

### Fixed
- `scripts/setup-deps.sh` still announced "Installing Voxtral inference server
  dependencies" and "MLX Voxtral dependencies installed". mlx-engine has been
  Kokoro-only since the engine was removed from the dictation path.

## [0.3.0] — 2026-09-22

The release that gave the shared constants one owner. A protocol spelled out
separately in Go, bash, Python and Swift is where most of this project's bugs
have come from; it is now generated from one file, and the copies that cannot
be generated fail a test when they disagree.

### Added
- `go install github.com/iksnerd/local-whisper/cmd/local-whisper@latest` is
  documented. It has worked since the module path was fixed; the README now
  also says what it leaves out — the model, the hooks, the menu bar app and the
  Kokoro server.
- `split_pattern` on `/speak`, the fifth and last parameter Kokoro's
  `generate()` accepts. Its default splits on blank lines, which is the wrong
  unit for one long paragraph; `"\. "` chunks by sentence.
- Copy and Save buttons on the live transcript page. Save downloads a
  timestamped `.txt` rather than sending you to find the log under `/tmp`.
- `make check-docs` fails when a Makefile target or `scripts/*.sh` is mentioned
  in no markdown file. It found five gaps on its first run, then caught itself.
- A pre-commit hook (`make install-hooks`) that checks the **staged snapshot**
  rather than the working tree, so a file importing something you never
  `git add`ed fails locally instead of on someone else's checkout.
- `docs/tuning.md` and `docs/architecture.md`.
- **`internal/protocol/protocol.json`, the one place the constants every runtime
  shares are written** — the TTS activity dir and its env override, the playback
  lock, the sidecar suffixes, mlx-engine's URL and pid file, the temp dir.
  `make generate-protocol` renders it into Go, bash, Swift and Python; `make
  check-protocol` fails when a rendered file is stale. It generates rather than
  having each runtime read one file at startup because `go:embed` cannot reach a
  parent directory, an installed binary has no `scripts/` beside it, and the app
  bundle carries only what it was told to.
- **`make check-names`** rejects a tracked filename that breaks another
  checkout: a character outside `A-Za-z0-9._-`, a leading dash, a Windows
  reserved device name, or two paths differing only by case.
- A test that reads the flags out of the Makefile's Raycast generator and
  asserts the CLI still defines them, so a generated launcher cannot outlive the
  flags it passes.
- **`local-whisper engine stop --keep-autostart`**, for a stop that is meant to
  be temporary. A plain `stop` writes `engineAutoStart=false` on purpose, but
  that setting outlives the command, and undoing a throwaway stop previously
  meant starting the server again — the opposite of putting things back.

### Changed
- **CI now runs only on version tags.** No checks on pull requests or pushes to
  `main`; the pre-commit hook is the pre-merge signal. `CONTRIBUTING.md` says so
  rather than implying a green tick that will not appear.
- `/speak` rejects unknown fields with a 422 instead of accepting them. A
  request carrying `temperature` used to return 200 and change nothing, because
  `mlx_audio`'s `generate_audio()` advertises ~25 parameters of which Kokoro
  accepts five.
- The missing-model error suggests `curl`, which macOS ships, instead of `wget`,
  which it does not.
- The README is a landing page: the Apple Silicon constraint moved above the
  install commands, the diagram moved to `docs/architecture.md`, and there is a
  section naming the dictation apps most readers should use instead.

### Fixed
- **`scripts/check-portable-paths.sh` was passing vacuously inside the
  pre-commit hook.** The hook exports a snapshot with no `.git`, `git ls-files`
  failed, the file list came back empty, and it printed its success line having
  examined nothing — a planted `/Users/<name>` passed. Both guards now fall back
  to `find` and refuse to report success on an empty list. The pattern is also
  case-insensitive; the old `[a-z]` pattern missed a capitalised username.
- A claim that whisper.cpp "cannot" stream, in eight places including two Go
  package comments. whisper.cpp ships `whisper-stream`; the accurate statement
  is about this project's wrapper, which transcribes a complete file per
  subprocess.
- `pkg/stt`'s package comment described a `Transcribe` method deleted when
  Voxtral left the dictation path.
- **`internal/speaker` and `internal/ttscontrol` had drifted apart while still
  agreeing on the value.** Both spelled `/tmp/ava-tts-active`, but only
  ttscontrol honoured `TTSCONTROL_ACTIVITY_DIR`, so the variable that exists to
  keep tests off the real shared directory redirected half the system. Both now
  resolve through one function.
- Three byte-identical `--server-url` help strings naming a port none of them
  derived from the client, and four spellings of `/tmp/voice-input`.
- **`engine stop` changed a persistent setting and said nothing.** It printed
  only "Server stopped" while turning hook auto-start off, so the way you found
  out was later, from hooks that had quietly started speaking through macOS
  `say`. It now names what it changed and how to undo it.
- **`make install-raycast` generated a command that could not run.** The two
  Voxtral Raycast generators went with the `--engine` flag; the whisper one
  kept emitting `--engine=whisper`, so the installed "Dictate with Whisper"
  command exited on `❌ unknown flag: --engine`. The flag survived the removal
  pass because the Makefile writes it through an `@echo` line, where it reads
  like prose rather than like a call site.
- Four more references to the removed engine: `CLAUDE.md` contradicted itself
  (one line says the flag is gone, another documented it), `AGENTS.md` called
  `pkg/mlx` one of "the two engines", `scripts/setup-model.sh` told you Voxtral
  models download on first run, and `mcp.go`'s doc comment still named Voxtral
  as something the CLI uses.
- A file whose name was a fragment of `check-portable-paths.sh` — a mistyped
  redirect turned a shell snippet into a filename, and it was committed. `|` is
  not legal on NTFS, so it broke a Windows checkout; neither guard in `make
  lint` looks at filenames, only at contents.

## [0.2.0] — 2026-09-22

The release that made the project fit to publish. Two engines became one, the
menu bar app got a name that isn't someone else's trademark, and several
failures that used to happen silently now say so.

### Added
- `--version` on both binaries, stamped from `git describe` at build time. The
  MCP handshake reports the same value instead of a hardcoded `0.1.0`.
- `local-whisper speak` warns on stderr when it falls back to the macOS `say`
  voice, which is the difference between "why does it sound robotic" being
  answerable and not.
- `--quiet`, as the inverse of `--verbose`, matching `a11y --quiet`.
- CI on ubuntu and macOS. No org secrets, so pull requests from forks run the
  same checks.
- `SECURITY.md`, `NOTICE`, `CONTRIBUTING.md`, and a Privacy and permissions
  section in the README covering what is written to `/tmp` and for how long.
- Tests for `mlx-engine`'s HTTP contract and `voxtral`'s filter and device
  resolution — the two components that had none. They stub the ML stack, so
  they need neither Apple Silicon nor the multi-gigabyte dependency tree.
- The transcript page reports the device, engine, language and log path it is
  using, and says when nothing has been transcribed yet.

### Changed
- **The menu bar app is now Ava.** Bundle id `xyz.iksnerd.ava`, config at
  `~/Library/Application Support/ava/config.json`, markers at
  `/tmp/ava-tts-*`. Shipping a `.dmg` named after someone else's product was
  trademark use as a product name.
- **The module path is `github.com/iksnerd/local-whisper`**, so `go install`
  works and `pkg/mlx` and `pkg/stt` are importable.
- A failed dictation exits non-zero and names the microphone permission, which
  is the most likely first-run failure and is silent when denied.
- `transcribe` distinguishes an undecodable file from a silent one instead of
  reporting both as "no speech detected".
- `speak --voice` validates against the known voices. An unknown id used to
  reach the engine, whose rejection is indistinguishable from the engine being
  down, so it spoke in a different voice and exited 0.
- `setup-deps.sh` skips the Apple-Silicon-only step on Intel rather than
  failing the whole setup at the last step.

### Removed
- **The Voxtral transcription engine and the `--engine` flag.** Measured on the
  same 20-second sample: whisper.cpp 1.26s against Voxtral's 17s warm and 127s
  cold, for a near-identical transcript. `mlx-engine` is now TTS-only —
  resident memory dropped from 4056 MB to 834 MB, the 11 GB load peak and the
  2.9 GB model download are gone, and the 15-minute idle shutdown no longer
  costs 108 seconds to undo. Voxtral still runs in `voice-monitor`, which needs
  streaming rather than a subprocess per complete file.

### Fixed
- `/events` lost any transcript delta broadcast between reading the history
  snapshot and subscribing. On a live call that is a silently dropped line.
- Reconnecting duplicated the entire transcript, because the history replay was
  indistinguishable from a delta and `EventSource` reconnects on any blip.
- The transcript could not be selected or copied while anyone was speaking.
- Autoscroll dragged the reader back to the bottom on every delta.
- Failures in the menu bar app were silent: exit codes were discarded and
  launch errors swallowed. The panel has an error surface now.
- Turning on LLM summary is verifiable — the panel says when Ollama is
  unreachable or the model is not pulled, instead of quietly truncating.
- VoiceOver announced both sliders as "slider, 50 percent" with no way to tell
  them apart.
- The idle shutdown deleted a pid file nothing wrote, leaving the real one.

## [0.1.0] — 2025-12-06

Initial release: the `local-whisper` CLI for dictation, with Raycast
integration.

[0.3.0]: https://github.com/iksnerd/local-whisper/releases/tag/v0.3.0
[0.2.0]: https://github.com/iksnerd/local-whisper/releases/tag/v0.2.0
[0.1.0]: https://github.com/iksnerd/local-whisper/releases/tag/v0.1.0
