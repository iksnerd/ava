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
make setup          # system deps, the Kokoro environment, the whisper model
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

**CI only runs on version tags**, so a pull request gets no check and a push to
`main` is not checked. Run the above yourself. The workflow runs on ubuntu and
macOS at release time.

`make install-hooks` points `core.hooksPath` at `.githooks/`, which covers the
gap: gofmt, build, vet, tests, ruff and the hardcoded-path check on every
commit. It runs against the **staged snapshot**, exported with
`git checkout-index`, not against your working directory — so staging a file
that imports something you never `git add`ed fails here rather than on someone
else's checkout. Docs-only commits skip the language checks and finish fast.
`git commit --no-verify` skips it when you mean to.

## Cutting a release

Pushing a `v*` tag starts CI. Once both test jobs pass, a third job builds the
`ava` binary with GoReleaser and attaches it to the release.

1. Move `CHANGELOG.md`'s `## [Unreleased]` section under the new version, and add
   its link at the bottom.
2. Commit, tag `vX.Y.Z`, push both.

`make release-snapshot` builds what the tag would publish and publishes nothing.
`make release-notes` prints the newest changelog section, which CI passes to
`--release-notes`; GoReleaser's commit-list generator is off.

The artifacts are **Apple Silicon only**: the code shells out to `afplay`,
`pbcopy`, `osascript` and `sox`, and the TTS engine is MLX. They are also
**unsigned**: macOS quarantines downloads, so a recipient runs
`xattr -d com.apple.quarantine` before the binary will start. Signing needs an
Apple Developer ID this project does not have, so the menu bar app is not
distributed this way.

## Things worth knowing before you change them

**One config file, three readers.** `~/Library/Application Support/ava/config.json`
is parsed independently by Go (`internal/voiceconfig`), bash (`scripts/lib.sh`)
and Swift (`VoiceSettings.swift`). They must agree, including on malformed
input. `internal/voiceconfig/contract_test.go` runs the bash and Go readers
against shared cases in `testdata/voice-config-cases.json`, and
`make check-swift-config` is the Swift arm. **Add a case there before changing
any reader.** A wrong-typed value must never silently unmute the app.

**Speech is a cross-process protocol, not a function call.** A global mute, an
activity marker in `/tmp/ava-tts-active`, and an exclusive `flock(2)` on
`/tmp/ava-tts-playback.lock` are shared by every `ava` process that speaks
(the CLI, the MCP server, and `scripts/speak.sh` for the hooks and the menu
bar app) and by `scripts/stop-speaking.sh`. Two of them speaking at once is the
failure this prevents. `internal/speaker`'s package comment documents the
contract.

**The hooks speak through `ava speak --async`.** `scripts/speak.sh` is only a
mute check plus a hand-off to it. Keep it that way: a second player drifts from the first.

**No hardcoded home directories.** `make lint` fails on `/Users/<name>` or
`/home/<name>` in any tracked file.

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
