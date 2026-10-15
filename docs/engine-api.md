# The engine API

[← Back to the README](../README.md)

`ava speak --out` is the supported way to get a WAV. This page is for a script that talks to the Kokoro server directly, and for anyone changing it. The server is `mlx-engine/server.py`, bound to `127.0.0.1:8765` and started with `ava engine start`.

Protocol: `ava-engine/2`. The server reports it in `/health`, and `ava` refuses an engine that reports another one. An engine left running across an upgrade is the usual way to meet that: `ava` says the running engine is older and to restart it with `ava engine stop && ava engine start`.

## `GET /health`

```json
{"status": "ok", "protocol": "ava-engine/2", "tts_model": "mlx-community/Kokoro-82M-bf16", "tts_loaded": true}
```

Answers only once the model has warmed up, so a 200 means `/speak` will work. It does not count as activity, so polling it does not keep an idle engine alive.

## `POST /speak`

A JSON body, answered with one `audio/wav` file.

| Field | Type | Default | |
|---|---|---|---|
| `text` | string | required | What to say. Must not be blank (400). Kokoro's inline pronunciation markup is passed through. |
| `voice` | string | `af_heart` | A voice id from `ava voices`, or several comma-separated to blend as an average. The first letter picks the language (`a` American English, `b` British, and so on). |
| `speed` | number | `1.0` | Rate multiplier. |
| `split_pattern` | string or null | null | Kokoro's chunking regex. The default splits on blank lines, which suits paragraphs and not one long block of text. See [tuning](tuning.md). |

Any other field is refused with 422 rather than ignored. Long text comes back as a single joined WAV, not one file per chunk. A synthesis failure is a 500 with the reason in `detail`. One synthesis runs at a time; a second request waits.

```bash
curl -s -X POST 127.0.0.1:8765/speak \
  -H 'Content-Type: application/json' \
  -d '{"text": "Once upon a time", "voice": "bf_emma", "speed": 0.95}' \
  -o scene1.wav
```

## `POST /speak/timed`

The same request body as `/speak`, answered with JSON that carries the audio and where each chunk of text sits in it:

```json
{
  "sample_rate": 24000,
  "segments": [
    {"text": "Once upon a time there was a lamp.", "start_ms": 0, "end_ms": 2000},
    {"text": "It glowed all night.", "start_ms": 2000, "end_ms": 3500}
  ],
  "audio": "<the WAV, base64>"
}
```

A segment is one piece of `split_pattern`, which is a line by default. For one segment per sentence send `"split_pattern": "(?<=[.!?])\\s+"`. Times are cumulative sample counts over the sample rate, so they are exact and do not drift. Errors are the same as `/speak`. `ava speak --timings` is the supported way to use it.

The server stops itself after 15 minutes without a `/speak` or `/speak/timed`.

## Changing it

Bump `engine_protocol` (it went to 2 when `/speak/timed` was added) in `internal/protocol/protocol.json`, then `make generate-protocol`, when `/speak` or `/health` change in a way an older client or engine cannot read. Adding a request field means documenting it here: a test fails when a `SpeakRequest` field is missing from this page.
