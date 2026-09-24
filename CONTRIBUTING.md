# Contributing

Bug reports and patches are welcome. This is a small, single-maintainer project,
so the honest expectation is slow but real responses.

If you are looking for somewhere to start, the
[`good first issue`](https://github.com/iksnerd/ava/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22)
label marks issues that name the files to touch, the existing pattern to follow,
and what "done" means.

## Getting set up

```bash
brew install sox whisper-cpp go
make setup          # system deps + the whisper model
make install-hooks  # pre-commit checks; see below for why this matters here
make test           # Go, mlx-engine, voxtral, and the voice hooks
```

`make test` needs no Apple Silicon and no MLX. Speech, call monitoring and the
menu bar app do.

## Before you open a PR

```bash
make lint           # go vet, gofmt, ruff, hardcoded paths, doc coverage
make test
make check-swift-config    # if you touched VoiceSettings.swift (needs swiftc)
```

`make test` runs everything. To narrow it while iterating: `make test-mlx-engine`
for the Kokoro server's HTTP contract, `make test-voxtral` for the filter and
device resolution. `make uninstall-hooks` turns the pre-commit hook back off.

**CI only runs on version tags here**, so a pull request will not get a green
tick and a push to `main` will not be checked. Run the above yourself; that is
the deal. The workflow still exists and still runs on ubuntu and macOS, it just
fires at release time.

`make install-hooks` points `core.hooksPath` at `.githooks/`, which fills the
gap: gofmt, build, vet, tests, ruff and the hardcoded-path check on every
commit. It runs against the **staged snapshot**, exported with
`git checkout-index`, not against your working directory — so staging a file
that imports something you never `git add`ed fails here rather than on someone
else's checkout. Docs-only commits skip the language checks and finish fast.
`git commit --no-verify` skips it when you mean to.

## Cutting a release

Tagging is the whole trigger. `v*` starts CI, and once both test jobs pass a
third job builds the two CLI binaries with GoReleaser and attaches them to the
release.

1. Move `CHANGELOG.md`'s `## Unreleased` section under the new version, and add
   its link at the bottom.
2. Commit, tag `vX.Y.Z`, push both.

`make release-snapshot` builds exactly what the tag would publish and publishes
nothing — worth running first, since the alternative is finding out from a
release. `make release-notes` prints the newest changelog section, which is
what CI feeds to `--release-notes`; GoReleaser's own commit-list generator is
off, so the hand-written notes are the only ones.

Two things about the artifacts. They are **Apple Silicon only** — the code
shells out to `afplay`, `pbcopy`, `osascript` and `sox`, and the TTS engine is
MLX, so a Linux build would compile and then fail at the first thing it did.
And they are **unsigned**: macOS quarantines anything downloaded, so a
recipient runs `xattr -d com.apple.quarantine` before the binary will start. Signing needs an Apple
Developer ID this project does not have, which is also why the menu bar app is
not distributed this way.

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
`/tmp/ava-tts-playback.lock` are shared by every `ava` process that speaks
(the CLI, the MCP server, and `scripts/speak.sh` for the hooks and the menu
bar app) and by `scripts/stop-speaking.sh`. Two of them speaking at once is the
failure this prevents. `internal/speaker`'s package comment documents the
contract.

**The hooks deliberately do not use the Go binary.** They are plain bash so
speech still works on a machine where `ava` was never installed. Keep
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

Include `ava --version`. If it's a speech problem, say whether you
heard the Kokoro voice or the macOS `say` fallback — they sound obviously
different, and it narrows the cause immediately.

Security issues go through
[private vulnerability reporting](https://github.com/iksnerd/ava/security/advisories/new),
not the issue tracker. See [SECURITY.md](SECURITY.md).
