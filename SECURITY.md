# Security

## Reporting a vulnerability

Use GitHub's private vulnerability reporting:
**[Report a vulnerability](https://github.com/iksnerd/local-whisper/security/advisories/new)**.

That keeps the report private until there's a fix, and it doesn't require
either of us to publish an email address. Expect a first response within a
week. If you'd rather not use GitHub, open a public issue saying only that you
have something to report and how to reach you — no details.

## What this project is, in security terms

There is no server, no account, and no telemetry. Everything runs as your user
on your machine: a Go CLI, a Swift menu bar app, and a local HTTP server bound
to `127.0.0.1`. Nothing is transmitted anywhere.

That means the interesting attack surface is local, and it is unusually
sensitive for a tool this small — it records audio, holds permission to
synthesise keystrokes, and can read other applications' accessibility trees.
The points worth your attention:

**`mlx-engine` accepts unauthenticated requests on `127.0.0.1:8765`.** Any
process running as your user can post text to `/speak` or audio to
`/transcribe`. There is no token. This is deliberate for a single-user local
service, but it means the port is as trusted as your user account. The same
applies to `voice-monitor`'s SSE server on `127.0.0.1:8766`, which serves a live
transcript to anything that connects.

**Accessibility permission is powerful.** Auto-paste drives Cmd+V through
AppleScript, which requires macOS Accessibility access. Granting it lets the
controlling process synthesise arbitrary keystrokes, not just Cmd+V. Grant it to
a terminal or app you trust, and use `local-whisper --no-paste` if you'd rather
not — the transcript still reaches your clipboard.

**Recorded audio and transcripts are written to `/tmp`.** See
[Privacy and permissions](README.md#privacy-and-permissions) in the README for
exactly what lands where, and for how long.

**Model weights are downloaded from Hugging Face at setup time.**
`scripts/setup-model.sh` fetches a whisper.cpp model over HTTPS and does not yet
verify a checksum, so it trusts Hugging Face and your TLS chain. Pinning a
revision and verifying a hash is tracked as a known gap.

**The `.app` is ad-hoc signed, not notarized.** There is no Apple Developer ID
behind it, so Gatekeeper will warn on any machine other than the one that built
it. Build it yourself from source if that matters to you.

## Supported versions

Pre-1.0 and single-maintainer: fixes land on `main`, and there are no backports.
