# Changelog

Notable changes. Dates are release dates; `v0.1.0` predates this file, so its
entry is a summary rather than a record kept as it happened.

## [Unreleased]

### Fixed
- `Ava.dmg` opened to a window holding only `Ava.app`, with no Applications shortcut to drag it
  onto. The shortcut is there now.

## [0.7.1] — 2026-09-24

Installing without Terminal: the menu bar app is on the release page as `Ava.dmg`, and its **Set
up Ava** button installs the rest. Homebrew is the one thing to install first. `ava setup` now also
installs `uv`, which it needed and never installed, and downloads Kokoro with every voice, so
speech works offline once setup is done.

### Added
- **`Ava.dmg` on the release page**, the menu bar app for installing without Terminal. It is
  built with the new `make release-dmg` and listed in `checksums.txt`. It is unsigned, so the
  first open needs System Settings → Privacy & Security → Open Anyway.
- **A Set up button in the menu bar app.** At launch the app runs its bundled `ava setup --check`,
  and if anything is missing it offers **Set up Ava**, which runs `ava setup` with its progress
  shown and goes away when done. Someone who installed only the app no longer needs Terminal,
  except to install Homebrew, which the card links to.
- **`ava setup --check`** reports what setup would still install, using setup's own rules, and
  installs nothing. It exits non-zero until setup is complete.

### Fixed
- `ava setup` never installed `uv`, which its engine step needs, so on a fresh Mac it stopped
  there with "install uv and re-run". It now installs `uv` through Homebrew with the other
  tools.

### Changed
- **`ava setup` downloads the Kokoro model**, weights and every voice, so a finished setup
  speaks with no network. It used to arrive on the first `ava speak`, and each voice separately
  the first time it was used, from a second repository. The engine now reads everything from
  one snapshot of `mlx-community/Kokoro-82M-bf16` at a pinned revision. `make setup` fetches it
  the same way, through the new `mlx-engine-server.sh fetch`.

## [0.7.0] — 2026-09-24

One binary. Live call transcripts are `ava monitor` instead of a separate `ava-monitor`, and the
names kept from before the 0.6.0 rename are gone. **Anything that runs `ava-monitor` needs to say
`ava monitor`**; nothing else changes.

### Changed
- **`ava-monitor` is now `ava monitor`**, and `ava-monitor devices` is `ava monitor devices`,
  with the same flags. It was a second binary that releases shipped although it cannot run
  without a checkout's Voxtral environment, and that `make install-bin` never updated. Releases
  and `install.sh` now carry `ava` alone, and `make build-ava-monitor` is gone: `make build`
  covers it. Delete an old `~/.local/bin/ava-monitor` by hand; nothing else uses it.

### Removed
- The command alias and environment-variable names kept from before the 0.6.0 rename, as
  announced for this release. `install.sh` and `make install-bin` no longer create
  the alias, `AVA_BIN` and `AVA_ENGINE_DIR` are the only names read, and `install.sh` installs
  0.6.0 and later only. An alias left by an earlier install still points at `ava`; delete it from
  `~/.local/bin` whenever you like.

## [0.6.3] — 2026-09-24

Fixes from a second review: speech that ignored a stop, a call monitor that looked connected after
its transcriber died or dropped text, and `--no-paste` skipping the copy. The voice hooks now speak
through `ava` itself, so **after pulling, run `make build` in the checkout the hooks point at** (or
install a release); the menu bar app bundles its own `ava` and needs nothing.

### Changed
- **`scripts/speak.sh` now speaks through `ava speak`.** It checks the mute and hands off to
  `ava speak --async`, instead of carrying its own synthesis, `say` fallback and player: a stop
  fix made to the Go player had missed the bash copy. The hooks and the menu bar's Test and Read
  Aloud therefore need `ava` built (`make build`) or installed; without it `speak.sh` exits
  non-zero and says how to get one.

### Fixed
- `ava-monitor` kept serving after `realtime.py` crashed, so the page read "connected" over a
  transcript that had stopped. It now exits with the transcriber's exit status, and the page
  turns red.
- A browser tab too slow to keep up had transcript updates silently dropped and stayed
  "connected" with gaps, and Copy and Save kept the gaps. The server now disconnects it, and
  the reconnect replaces the page with the full transcript.
- `ava --no-paste` skipped the clipboard copy as well as the paste, while it printed "Copied"
  and the docs said the transcript stays on the clipboard. It now copies.
- A stop that landed while a speak was taking the playback lock found no player to kill, and
  the speech played in full. The speak now checks for the stop on both sides of starting the
  player.
- `speak.sh` (the hooks and the menu bar) had the same stop race, and a stop landing just after
  afplay started killed only its wrapper and left afplay playing. It now uses the Go player.
- A monitor tab opened before the model was ready kept saying "Waiting for the session to
  start…". The session is now sent to tabs already connected when it arrives.
- Every `speak.sh` speak left an empty file in the per-user temp folder. `speak.sh` no longer
  writes audio files of its own.

## [0.6.2] — 2026-09-23

Installing from a release, for someone who has never seen the project: a README Install section
led by a one-line install, a new-Mac guide, and `--help` that says where to start.

### Added
- **`docs/getting-started.md`**, a new-Mac checklist with a check after every step: the release
  install first, the full source install (menu bar app, voice hooks, call transcripts) after.
- `ava --help` opens with the first-run sequence (`ava setup`, `ava speak "hello"`, `ava`) and
  the two permission prompts; `ava-monitor --help` says it needs the Voxtral environment from a
  checkout.

### Changed
- The README's Quick start is now **Install**: from a release with
  `curl -fsSL https://raw.githubusercontent.com/iksnerd/ava/main/scripts/install.sh | bash`,
  a manual download from the Releases page (checksum and quarantine), then from source.

### Fixed
- `install.sh` used `gh` whenever it was installed, and a `gh` that was never signed in refuses
  even public downloads, so the install failed with an auth error. It now uses `gh` only when
  signed in and `curl` otherwise.

## [0.6.1] — 2026-09-23

The release from an architecture and test review. The worst of it: a stop
could kill `ava` itself, including a running `ava mcp`; dictating deleted a
live call's transcript; the menu bar app never used Kokoro unless something
else had started it; and `ava engine stop` killed any local FastAPI app
started as `uvicorn server:app`. Each fix landed with a test that fails
without it.

### Fixed
- **A stop could kill `ava` itself, including a running `ava mcp`.** While
  waiting its turn to play, the Go speaker wrote its own PID into the file
  every stop signals, so the menu bar's Stop, `ava stop` or a dictation
  starting killed it, and left the menu bar showing "speaking". It now waits
  for the stop marker instead, and a stop never signals its own process.
- **Dictation deleted `ava-monitor`'s call transcripts.** It cleaned up by
  removing the whole shared temp directory, including a live call's log.
  Each dictation now gets its own directory and removes only that.
- **`ava speak --async` did nothing.** The process exited before the
  background speech started. It now hands the speech to a detached copy of
  itself.
- The MCP `speak` and `speak_accessibility_tree` tools and `ava a11y --voice`
  now reject an unknown voice, as `ava speak` does, instead of speaking in
  the macOS voice and reporting success; MCP `transcribe` checks the file
  exists first.
- `ava engine stop` no longer runs `pkill -f "uvicorn server:app"`, which
  killed any FastAPI app on the machine started that way. It stops the
  server with SIGTERM, checks a pid file's PID is really the server, and
  `status` reports a server that answers `/health` as running.
- The engine answers `/health` while it is speaking. Synthesis blocked the
  event loop, so a second speaker during a long sentence decided the engine
  was down and used the macOS voice, and the menu bar showed Stopped.
- `ava engine` and speech auto-start find the engine script the same way;
  `ava engine` ignored `MLX_ENGINE_SCRIPT`, so the two could drive different
  scripts.
- The menu bar app saves only the settings you change. It wrote every
  setting, freezing all defaults into your config so later default changes
  never reached you, and could overwrite a setting another process had just
  written.
- `speak.sh` passed a wrong-typed `sayRate` or `voice` from the config
  straight through; it now type-checks them like the Go reader does.
- The live transcript page refuses requests whose `Host` is not localhost,
  so a web page cannot read it through DNS rebinding.
- A failed speech auto-start says why, not just "exit status 1", and
  whisper-cli runs under a 30-minute timeout instead of none.
- The `.dmg` no longer carries the build machine's checkout path.
- **The menu bar app never used Kokoro unless a server was already running.**
  Its bundled copy of `scripts/` looked for `mlx-engine/` inside the app,
  never found it, and its Start button waited two minutes before failing, so
  every word came out in the macOS `say` voice. `build-app.sh` now records the
  checkout the app was built from, and the start script looks there, then in
  the engine `ava setup` installed. With neither it fails at once and says
  what to run, and a server that dies during startup is reported immediately
  instead of after the timeout.
- `MLX_ENGINE_PID_FILE` was never actually passed to the server: a comment
  after a line continuation turned it into a plain shell variable. Harmless so
  far only because the server's default is the same path.

### Changed
- The pre-commit hook runs the Go and Python suites when only scripts
  change (tests in both read the scripts), and syntax-checks every script.
  CI is tag-only, so a scripts-only commit used to land untested.
- `scripts/mlx-engine-server.sh` is now tested as a process against temp
  layouts (checkout, Ava.app, installed bundle, none) through `AVA_ENGINE_*`
  test-hook overrides, and speech auto-start, `ava setup`'s engine step and
  undecodable audio have tests where they had none.

## [0.6.0] — 2026-09-22

The project is called Ava now, after the menu bar app it already shipped. The
repository is `iksnerd/ava`, the commands are `ava` and `ava-monitor`, and the Go
module path is `github.com/iksnerd/ava`. Nothing else moves: the config, the
engine install and the speech markers already lived under `ava`.

### Changed
- **`local-whisper` is now `ava`, and `voice-monitor` is now `ava-monitor`.**
  `scripts/install.sh` and `make install-bin` also create a `local-whisper`
  symlink, so an existing MCP registration, Raycast launcher or Ava.app keeps
  working. The alias goes away in 0.7.0.
- The MCP server introduces itself as `ava`, so its tools appear as
  `mcp__ava__*` once registered under that name.
- `AVA_ENGINE_DIR` and `AVA_BIN` replace `LOCAL_WHISPER_ENGINE_DIR` and
  `LOCAL_WHISPER_BIN`; the old names are still read until 0.7.0.
- `make install-bin` installs with `install` rather than `cp`. `cp` rewrites
  the file in place, and macOS kills a running `ava mcp` whose binary changes
  under it.
- `make uninstall` also removes the pre-rename binaries and the alias.

### Fixed
- Release pages published with only the install footer for a body (v0.4.0 and
  v0.5.0, both repaired by hand). `changelog.disable: true` in
  `.goreleaser.yaml` skips the pipe that reads `--release-notes`, so the notes
  file CI builds from this changelog was never used.

### Upgrading from local-whisper
```bash
bash scripts/install.sh            # or: make install-bin
claude mcp remove -s user local-whisper
claude mcp add -s user ava \
  -e MLX_ENGINE_SCRIPT="$PWD/scripts/mlx-engine-server.sh" \
  -- "$HOME/.local/bin/ava" mcp    # drop the -e line if you ran `ava setup`
make install-raycast               # if you use the Raycast launcher
```
GitHub redirects the old repository URL, but update your remote with
`git remote set-url origin https://github.com/iksnerd/ava.git`. The voice hooks
in `~/.claude/settings.json` point at your checkout's folder, so they only need
changing if you rename that folder.

## [0.5.0] — 2026-09-22

The release that closes what a pre-launch audit found. `voice-monitor` no
longer serves a live call transcript to the local network, the engine's
dependencies and the Go toolchain are past their known advisories, and the
model download is pinned and checksummed. A release install now speaks in the
Kokoro voice it installed rather than the macOS fallback.

### Added
- **`local-whisper setup-model [--model base|tiny]`** downloads just the
  whisper.cpp model, including `tiny`, which `setup` never fetched. The
  model-not-found error now names it instead of printing a `curl` line.
- **`--beam-size`** on dictation and `transcribe`, and `beam_size` on the MCP
  `transcribe` tool: whisper.cpp's beam width, passed only when set. Lowering
  it trades accuracy for speed, mostly worth it on `tiny`.
- **`docs/uninstall.md`** and **`make uninstall`**. The target removes what only
  this project uses (the binaries, the Raycast script, the engine bundle) and
  lists the rest, never touching the shared Hugging Face cache.

### Changed
- The live transcript page tells a dropped connection from a stopped monitor.
  An error shows amber **reconnecting…** while `EventSource` retries; only
  after 10 seconds without reconnecting does it turn red, now labelled
  "transcript stopped", with a ⚠ in the tab title. Both used to be the same red
  "disconnected".
- Model downloads, from `setup`, `setup-model` and `scripts/setup-model.sh`,
  come from a pinned Hugging Face revision and are checked against its sha256.
  They used to fetch `resolve/main`, a mutable ref, and check nothing.

- Builds use Go 1.26.8 through a `toolchain` line in `go.mod`, which CI's
  `setup-go` honours. v0.4.0 was built with Go 1.25.0, a release now out of
  support, and `govulncheck` found standard-library advisories reachable
  through the model download and the engine client. It now reports none. The
  minimum Go for `go install` stays 1.25.
- The release job uses `goreleaser-action@v7`; v6 ran on the retired Node 20.

### Fixed
- **`voice-monitor` served the live transcript on every network interface.**
  It listened on `:8766`, so anyone on the same network could read a call
  transcript with no authentication, while the README and `SECURITY.md` said
  it stayed on `127.0.0.1`. It now binds loopback only.
- mlx-engine's lockfile pinned versions with published advisories, including
  `starlette` 1.0.0 and `python-multipart` 0.0.22, which serve the engine's
  local port. Upgraded to 1.6.0 and 0.0.32, with `transformers`, `urllib3`,
  `anyio`, `click`, `idna` and `msgpack`; `pip-audit` now reports nothing.
  The copy embedded in the binary is regenerated, so `local-whisper setup`
  installs the same set.
- After `local-whisper setup`, `speak` still could not auto-start the engine it
  had just installed: auto-start looked for the control script only relative to
  the working directory, so a release install spoke every line in the macOS
  voice unless `MLX_ENGINE_SCRIPT` was set. It now also finds the installed
  bundle, in the same order `local-whisper engine` uses.
- The release job published a body containing only the footer. GoReleaser's
  `release.mode` defaults to `keep-existing`, which declines to set the body;
  it is now `replace`, since CI supplies the body from `CHANGELOG.md`
  deliberately. The CI step also refuses an empty notes file, because the two
  causes are indistinguishable after the fact and only one of them was proven.

## [0.4.0] — 2026-09-22

The release that makes the binary self-sufficient. `local-whisper setup`
installs the dependencies, the speech model and the Kokoro engine — which now
travels inside the binary — so a machine with no checkout gets the real voice
rather than the macOS fallback. Tagged builds publish binaries.

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
- **GoReleaser builds the two CLI binaries for a tag** (`.goreleaser.yaml`), and
  a third CI job attaches them to the release once both test jobs pass.
  `darwin/arm64` only, deliberately: the code shells out to macOS tools and the
  TTS engine is MLX. `make release-snapshot` builds the same artifacts locally
  without publishing, and `make release-notes` prints the newest changelog
  section, which is what CI hands to `--release-notes`.
- **`scripts/install.sh`** installs the CLI binaries from a release, verifying
  the checksum before extracting. It fetches with `gh` or `curl`, neither of
  which attaches `com.apple.quarantine` — a browser does, and the binaries are
  unsigned, so a browser download is killed with exit 137 and no message while
  the dialog macOS shows offers **Move to Trash** rather than Open. Measured on
  macOS 27, including that quarantine survives `tar xzf`, so extracting in a
  terminal does not launder it.


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

[0.7.1]: https://github.com/iksnerd/ava/releases/tag/v0.7.1
[0.7.0]: https://github.com/iksnerd/ava/releases/tag/v0.7.0
[0.6.3]: https://github.com/iksnerd/ava/releases/tag/v0.6.3
[0.6.2]: https://github.com/iksnerd/ava/releases/tag/v0.6.2
[0.6.1]: https://github.com/iksnerd/ava/releases/tag/v0.6.1
[0.6.0]: https://github.com/iksnerd/ava/releases/tag/v0.6.0
[0.5.0]: https://github.com/iksnerd/ava/releases/tag/v0.5.0
[0.4.0]: https://github.com/iksnerd/ava/releases/tag/v0.4.0
[0.3.0]: https://github.com/iksnerd/ava/releases/tag/v0.3.0
[0.2.0]: https://github.com/iksnerd/ava/releases/tag/v0.2.0
[0.1.0]: https://github.com/iksnerd/ava/releases/tag/v0.1.0
