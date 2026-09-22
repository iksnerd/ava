# Changelog

Notable changes. Dates are release dates; `v0.1.0` predates this file, so its
entry is a summary rather than a record kept as it happened.

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
  costs 108 seconds to undo. Voxtral still runs in `voice-monitor`, where it
  streams at under 500ms, which whisper.cpp cannot do.

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

[0.2.0]: https://github.com/iksnerd/local-whisper/releases/tag/v0.2.0
[0.1.0]: https://github.com/iksnerd/local-whisper/releases/tag/v0.1.0
