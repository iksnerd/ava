# Security

## Reporting a vulnerability

Use GitHub's private vulnerability reporting:
**[Report a vulnerability](https://github.com/iksnerd/ava/security/advisories/new)**.

The report stays private until there is a fix. Expect a first response within
a week. If you would rather not use GitHub, open a public issue saying only that
you have something to report and how to reach you, with no details.

## What this project is, in security terms

There is no server, no account, and no telemetry. Everything runs as your user
on your machine: a Go CLI, a Swift menu bar app, and a local HTTP server bound
to `127.0.0.1`. Nothing is transmitted anywhere.

The attack surface is local: the tool records audio, can synthesise keystrokes
and can read other applications' accessibility trees. Points to know:

**`mlx-engine` accepts unauthenticated requests on `127.0.0.1:8765`.** Any
process running as your user can post text to `/speak` and have it spoken. There
is no token; the port is as trusted as your user account. The same applies to
`ava monitor`'s SSE server on `127.0.0.1:8766`, which serves a live transcript
to anything that connects.

**Accessibility permission is powerful.** Auto-paste drives Cmd+V through
AppleScript, which requires macOS Accessibility access. Granting it lets the
controlling process synthesise arbitrary keystrokes, not just Cmd+V. Grant it to
a terminal or app you trust, and use `ava --no-paste` if you'd rather
not; the transcript still reaches your clipboard.

Recorded audio and transcripts are written to `/tmp`; see
[Privacy](README.md#privacy) for what lands where and for how long.

**Model weights are downloaded from Hugging Face.** The whisper.cpp model is
pinned to a Hugging Face commit and checked against a sha256 before it is
installed, by both `ava setup-model` and `scripts/setup-model.sh`, so
a changed file on Hugging Face fails the download rather than landing on disk.
The Kokoro weights are pinned to a Hugging Face revision
(`TTS_MODEL_REVISION` in `mlx-engine/server.py`) but not checksummed. The
weights `ava monitor` uses (Voxtral, the Whisper fallback and the Sortformer
diarizer) are not pinned at all: `voxtral/realtime.py` names them by
repository id only. In both cases the Python libraries fetch them into the
shared Hugging Face cache the first time the server or `ava monitor` loads
them, trusting Hugging Face and your TLS chain.

**Releases are not Developer ID signed or notarized.** The `.app` is ad-hoc
signed and the binaries are unsigned, so macOS asks you to confirm the first
launch. Build from source if that matters to you.

## Supported versions

Pre-1.0 and single-maintainer: fixes land on `main`, and there are no backports.
