# ava monitor: live call transcripts

[← Back to the README](../README.md)

`ava monitor` streams a running transcript of the audio it is pointed at, serves
it at `http://localhost:8766` over SSE, and writes it to a log file. Point it at
a microphone for a room, or at a loopback device to capture a video call.

Apple Silicon only (it runs through MLX). See
[`voxtral-architecture.mmd`](voxtral-architecture.mmd) for how the pieces fit.

## Prerequisites

The monitor needs a source checkout; a release binary alone is not enough.

```bash
make setup-voxtral          # uv sync into voxtral/.venv; re-run after editing voxtral/pyproject.toml
make build                  # bin/ava; run the commands below from the checkout's root
```

To capture a call rather than a microphone you also need a loopback device.
`make setup-blackhole` installs [BlackHole
2ch](https://github.com/ExistentialAudio/BlackHole) and prints the remaining
steps. Two things to know:

- **Installing the driver needs a reboot** before macOS lists it as a device.
- **Build the Multi-Output Device by hand** in Audio MIDI Setup. Combine your
  normal output (built-in speakers) with BlackHole 2ch, so you hear the call and
  it reaches the capture path.

BlackHole is licensed GPL-3.0 and is installed separately by Homebrew; it is
not distributed with this project.

## Running it

```bash
# Voxtral Realtime: <500ms latency, 13 languages
bin/ava monitor --device BlackHole

# Whisper: ~1s latency, 99+ languages
bin/ava monitor --device BlackHole --engine whisper --language de
```

Open `http://localhost:8766`. The transcript is also written live to
`/tmp/voice-input/transcript-<timestamp>.txt`.

The page has **Copy** and **Save** buttons (Save downloads a timestamped
`.txt`). The badge beside the title shows **connected**, **reconnecting…** or,
after 10 seconds without a stream, **disconnected**, which
means the monitor is no longer transcribing. The server exits on Ctrl-C or when
`realtime.py` dies; the terminal says why.

`bin/ava monitor devices` lists audio devices and their indices. Indices shift
when devices are added or removed, so check them again if capture stops.

### Choosing an engine

`--engine voxtral` (the default) is the low-latency path and covers EN, FR, ES,
DE, RU, ZH, JA, IT, PT, NL, AR, HI, KO. For any other language use
`--engine whisper --language <code>`: 99+ languages at roughly 1s latency.

### Flags

| Flag | Default | Effect |
| --- | --- | --- |
| `--device` | system default input | Audio input device, by name or index |
| `--engine` | `voxtral` | `voxtral` (fast) or `whisper` (more languages) |
| `--language` | auto | Language code, e.g. `de`. `--engine whisper` only |
| `--diarize` | off | Tag deltas with `[Speaker N]`. `--engine whisper` only. See the licence note below |
| `--highpass-hz` | `0` (80Hz) | High-pass cutoff in Hz before STT. `0` selects the 80Hz default, not no filter |
| `--stt-model` | per engine | Override the STT model id |
| `--port` | `8766` | HTTP/SSE port |
| `--log` | `/tmp/voice-input/transcript-<ts>.txt` | Transcript log path |
| `--python` | `<voxtral-dir>/.venv/bin/python3` | Python interpreter to use |
| `--voxtral-dir` | `voxtral/` | Location of the Python primitives |

## `--diarize` uses a non-commercial model

Speaker diarization runs
[`mlx-community/diar_sortformer_4spk-v1-fp32`](https://huggingface.co/mlx-community/diar_sortformer_4spk-v1-fp32),
derived from `nvidia/diar_sortformer_4spk-v1`, which is licensed
**CC-BY-NC-4.0, non-commercial use only**. Every other model this project
uses is Apache-2.0 or MIT. If you are transcribing calls for commercial work,
do not pass `--diarize` without checking that licence yourself.

## Troubleshooting: the capture device reads silence

If BlackHole is installed and selected but nothing is transcribed, check in
order:

1. BlackHole is ticked in the Multi-Output Device (re-check after any edit in
   Audio MIDI Setup).
2. No Bluetooth device is in the Multi-Output Device. Use a wired or built-in
   output.
3. The conferencing app has the Multi-Output Device selected in its own audio
   settings. Apps keep the output device they had when you joined and ignore
   later changes to the system default (Google Meet: three-dot menu → Settings →
   Audio → Speakers).

To see whether audio reaches the device at all, replace `INDEX` with the index
from `bin/ava monitor devices`:

```bash
cd voxtral && uv run python3 -c "
import sounddevice as sd, numpy as np
rec = sd.rec(int(4*16000), samplerate=16000, channels=1, dtype='float32', device=INDEX)
sd.wait()
print(f'peak={np.abs(rec).max():.6f} rms={np.sqrt(np.mean(rec**2)):.6f} non-zero={np.count_nonzero(rec)}/{rec.size}')
"
```

`peak=0.000000` means no audio reaches the device, and STT tuning will not help.

## Known limitations

- **Repeated phrases.** The streaming decoder sometimes rewords part of a phrase
  between updates, and the result is emitted twice. The decoder is reset every
  20 seconds to limit it.
- **`--diarize` cannot label your own microphone.** It separates only the remote
  participants in the call audio, because your mic does not pass through the
  loopback.
- **No function calling.** The transcriber returns plain text only.
