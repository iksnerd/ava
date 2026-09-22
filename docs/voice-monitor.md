# voice-monitor — live call transcripts

[← Back to the README](../README.md)

`voice-monitor` streams a running transcript of whatever audio it is pointed
at, serves it at `http://localhost:8766` over SSE, and writes it to a log file.
Point it at your microphone for a room, or at a loopback device to capture a
video call.

Apple Silicon only — it runs through MLX. See
[`voxtral-architecture.mmd`](voxtral-architecture.mmd) for how the pieces fit.

## Prerequisites

```bash
make setup-voxtral          # uv sync into voxtral/.venv; re-run after editing voxtral/pyproject.toml
make build-voice-monitor
```

To capture a call rather than a microphone you also need a loopback device.
`make setup-blackhole` installs [BlackHole
2ch](https://github.com/ExistentialAudio/BlackHole) and prints the remaining
steps. Two things to know:

- **Installing the driver needs a reboot** before macOS lists it as a device.
- **The Multi-Output Device has to be built by hand** in Audio MIDI Setup —
  that part isn't scriptable. Combine your normal output (built-in speakers)
  with BlackHole 2ch, so you hear the call *and* it reaches the capture path.

BlackHole is licensed GPL-3.0 and is installed separately by Homebrew; it is
not distributed with this project.

## Running it

```bash
# Voxtral Realtime: <500ms latency, 13 languages
bin/voice-monitor --device BlackHole

# Whisper: ~1s latency, 99+ languages
bin/voice-monitor --device BlackHole --engine whisper --language de
```

Open `http://localhost:8766` and watch the transcript. It is also written live
to `/tmp/voice-input/transcript-<timestamp>.txt`.

The page has **Copy** and **Save** buttons in the header. Save downloads a
timestamped `.txt`, so it is the quickest way to keep a transcript from a
machine where you would rather not go hunting through `/tmp`. Copy needs a
secure context — over plain `http://localhost` browsers allow it, but it reports
"Blocked" rather than failing silently if yours does not.

`bin/voice-monitor devices` lists audio devices and their indices. Indices
shift whenever devices are added or removed — including disconnecting
headphones — so check them again if capture stops working.

### Choosing an engine

`--engine voxtral` (the default) is the low-latency path and covers EN, FR, ES,
DE, RU, ZH, JA, IT, PT, NL, AR, HI, KO. If your language isn't on that list, no
Voxtral model covers it — use `--engine whisper --language <code>` instead,
which covers 99+ languages at roughly 1s latency.

### Flags

| Flag | Default | Effect |
| --- | --- | --- |
| `--device` | system default input | Audio input device, by name or index |
| `--engine` | `voxtral` | `voxtral` (fast) or `whisper` (more languages) |
| `--language` | auto | Language code, e.g. `de`. `--engine whisper` only |
| `--diarize` | off | Tag deltas with `[Speaker N]`. `--engine whisper` only. **See the licence note below** |
| `--highpass-hz` | `0` → 80Hz | High-pass cutoff before STT. `0` means "use the 80Hz built-in default", it does not disable the filter. Loopback audio is much quieter than mic input, so the noise floor matters more |
| `--stt-model` | per engine | Override the STT model id |
| `--port` | `8766` | HTTP/SSE port |
| `--log` | `/tmp/voice-input/transcript-<ts>.txt` | Transcript log path |
| `--python` | `voxtral/.venv` | Python interpreter to use |
| `--voxtral-dir` | `voxtral/` | Location of the Python primitives |

## `--diarize` uses a non-commercial model

Speaker diarization runs
[`mlx-community/diar_sortformer_4spk-v1-fp32`](https://huggingface.co/mlx-community/diar_sortformer_4spk-v1-fp32),
derived from `nvidia/diar_sortformer_4spk-v1`, which is licensed
**CC-BY-NC-4.0 — non-commercial use only**. Every other model this project
uses is Apache-2.0 or MIT. If you are transcribing calls for commercial work,
do not pass `--diarize` without checking that licence yourself.

## Troubleshooting: the capture device reads pure silence

If BlackHole is installed and selected and you still get nothing, the symptom
is distinctive: recording off the BlackHole device index reads **bit-exact
zero** — not quiet, literally silent. Check these in order.

1. **BlackHole is in the Multi-Output Device's checked device list.** Obvious,
   but re-verify after any edit in Audio MIDI Setup.
2. **No Bluetooth device in the Multi-Output Device.** Wireless outputs are
   flaky here — audio plays fine while never reaching the capture path. Use a
   wired or built-in output.
3. **The conferencing app caches its own output device at join time** and does
   not follow the macOS system default changing afterward. This is the most
   common root cause once the device itself is configured correctly. Fix it
   inside the call's own audio settings by explicitly selecting the
   Multi-Output Device — in Google Meet that's three-dot menu → Settings →
   Audio → Speakers.

To confirm whether audio is arriving at all:

```bash
cd voxtral && uv run python3 -c "
import sounddevice as sd, numpy as np
rec = sd.rec(int(4*16000), samplerate=16000, channels=1, dtype='float32', device=INDEX)
sd.wait()
print(f'peak={np.abs(rec).max():.6f} rms={np.sqrt(np.mean(rec**2)):.6f} non-zero={np.count_nonzero(rec)}/{rec.size}')
"
```

Replace `INDEX` with the device index from `bin/voice-monitor devices`. A
`peak` of `0.000000` with `non-zero=0/64000` means audio is not reaching the
device, and no amount of STT tuning will help.

## Known limitations

- **Phrase repetition within a segment.** The streaming decoder occasionally
  re-decodes and rewords part of a phrase between consecutive ticks, and the
  length-based token diff doesn't recognise that as a revision, so it gets
  emitted twice. Mitigated — not fixed — by resetting the decoder every
  `MAX_SEGMENT_SECONDS` (20s) in `voxtral/realtime.py::run_whisper`. A real fix
  needs longest-common-prefix diffing, or surfacing provisional text that gets
  replaced, the way commercial live-caption UIs do.
- **`--diarize` can never label your own microphone.** It distinguishes only
  the remote participants already mixed into the call's audio output, because
  your mic never flows through the loopback capture path. Full "who said what,
  including me" would need a second mic-capture path merged in.
- **No function calling.** The local STT wrapper returns plain text only, so
  voice-driven actions would need a separate intent layer on top.
