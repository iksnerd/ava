# Contributing

Bug reports and patches are welcome. This is a small, single-maintainer project,
so the honest expectation is slow but real responses.

## Getting set up

```bash
brew install sox whisper-cpp go
make setup          # system deps + the whisper model
make test           # Go, mlx-engine, voxtral, and the voice hooks
```

`make test` needs no Apple Silicon and no MLX. Speech, call monitoring and the
menu bar app do.

## Before you open a PR

```bash
make lint           # go vet, gofmt, ruff, and the hardcoded-path check
make test
make check-swift-config    # if you touched VoiceSettings.swift (needs swiftc)
```

CI runs the same things on ubuntu and macOS. It needs no secrets, so a PR from
a fork gets the same green tick you do.

## Things worth knowing before you change them

**One config file, three readers.** `~/Library/Application Support/ava/config.json`
is parsed independently by Go (`internal/voiceconfig`), bash (`scripts/lib.sh`)
and Swift (`VoiceSettings.swift`). They must agree, including on malformed
input. `internal/voiceconfig/contract_test.go` runs the bash and Go readers
against shared cases in `testdata/voice-config-cases.json`, and
`make check-swift-config` is the Swift arm. **Add a case there before changing
any reader.** The one config bug that ever shipped was a wrong-typed value
silently unmuting the app, in the component that had no test.

**Speech is a cross-process protocol, not a function call.** A global mute, an
activity marker in `/tmp/ava-tts-active`, and an exclusive `flock(2)` on
`/tmp/ava-tts-playback.lock` are shared by the Go binary, `scripts/speak.sh`,
the menu bar app and the Claude Code hooks. Two of them speaking at once is the
failure this prevents. `internal/speaker`'s package comment documents the
contract.

**The hooks deliberately do not use the Go binary.** They are plain bash so
speech still works on a machine where `local-whisper` was never installed. Keep
it that way.

**No hardcoded home directories.** `make lint` fails on `/Users/<name>` or
`/home/<name>` in any tracked file. This caught two real bugs.

## Style

Match the surrounding code. Go is `gofmt`-clean and `go vet`-clean; Python is
`ruff`-formatted; commit subjects are plain sentences describing what changed,
not `type(scope):` prefixes — `git log` shows the pattern.

Tests should fail for the reason they claim. If you add one, try breaking the
code it guards and check that it actually goes red.

## Reporting bugs

Include `local-whisper --version`. If it's a speech problem, say whether you
heard the Kokoro voice or the macOS `say` fallback — they sound obviously
different, and it narrows the cause immediately.

Security issues go through
[private vulnerability reporting](https://github.com/iksnerd/local-whisper/security/advisories/new),
not the issue tracker. See [SECURITY.md](SECURITY.md).
