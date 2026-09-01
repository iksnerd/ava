# Voxtral call-monitor setup

See `docs/voxtral-architecture.mmd` for the full architecture this fits into.

## Done
- [x] `pkg/realtimestt` (Go wrapper around `realtime.py`) + `voxtral/` (MLX primitives: `realtime.py`), managed as a uv project (`voxtral/pyproject.toml` + `uv.lock`) — `make setup-voxtral` runs `uv sync` into `voxtral/.venv`
- [x] `cmd/voice-monitor` — serves live transcript at `localhost:8765` (SSE) and logs it to `/tmp/voice-input/transcript-<timestamp>.txt`
- [x] BlackHole 2ch installed (`brew list --cask blackhole-2ch`) + Multi-Output Device ("Meet Recording") in Audio MIDI Setup, built from **MacBook Pro Speakers + BlackHole 2ch** (not Bluetooth/AirPods — see gotcha below)
- [x] Dual STT engine in `realtime.py`/`pkg/realtimestt`: `--engine voxtral` (default, <500ms, 13 languages) or `--engine whisper` (multilingual, 99+ languages, ~1s latency)
- [x] `--diarize` — tags transcript deltas with `[Speaker N]` via Sortformer (mlx-audio, no extra dep), whisper engine only
- [x] `--highpass-hz` (default 80Hz) — cuts low-frequency rumble/bass before STT/diarization; important because BlackHole loopback audio runs much quieter than mic input, so noise floor matters more
- [x] End-to-end confirmed working against a real Bulgarian Meet call, with speaker labels

## Gotcha: getting audio into BlackHole at all (the actual saga)
Confirming BlackHole is installed and selected as the macOS system output is **not enough**. In order, what actually had to be true:
1. **BlackHole must be in the Multi-Output Device's checked device list** (obvious, but re-verify after any edit).
2. **Avoid Bluetooth (AirPods) as a device in the Multi-Output Device** — flaky in practice; audio plays fine but can silently never reach the other member. Use MacBook Pro Speakers instead. (Note: even after swapping to speakers, that alone did not fix it — see #3.)
3. **Google Meet caches its own audio-output device at call-join time** and does not follow macOS's system-default output changing afterward. This was the actual root cause once BlackHole/Multi-Output were correctly configured. Fix: inside the Meet call itself, three-dot menu → Settings → Audio tab → Speakers dropdown → explicitly select the Multi-Output Device.

Symptom throughout: `sd.rec()` off the BlackHole device index reads bit-exact zero (`peak=0.0, non-zero=0/N`) — not just quiet, literally silent — whenever any of the above isn't right. Useful one-liner to check:
```
cd voxtral && uv run python3 -c "
import sounddevice as sd, numpy as np
rec = sd.rec(int(4*16000), samplerate=16000, channels=1, dtype='float32', device=<blackhole-index>)
sd.wait()
print(f'peak={np.abs(rec).max():.6f} rms={np.sqrt(np.mean(rec**2)):.6f} non-zero={np.count_nonzero(rec)}/{rec.size}')
"
```
(Get `<blackhole-index>` from `bin/voice-monitor --list-devices` — it shifts when devices are added/removed, e.g. after unplugging/disconnecting AirPods.)

## Known finding: Bulgarian is not covered by any Voxtral model
Checked all three model cards - none list Bulgarian:
- Voxtral Mini 3B (STT): EN, FR, DE, ES, IT, PT, NL, HI
- Voxtral Mini 4B Realtime: EN, FR, ES, DE, RU, ZH, JA, IT, PT, NL, AR, HI, KO
- Voxtral 4B TTS: EN, FR, ES, DE, IT, PT, NL, AR, HI

Use the Whisper engine instead (OpenAI Whisper covers 99+ languages via `mlx-audio`, code `bg`). Higher latency (~1s chunked, vs Voxtral Realtime's <500ms) but actually transcribes Bulgarian, unlike Voxtral.

## Run it against a live call
```
make setup-voxtral        # first time only (or after editing voxtral/pyproject.toml)
make build-voice-monitor
bin/voice-monitor --device BlackHole --engine whisper --language bg --diarize
```
Open `http://localhost:8765`, join the Meet call (with Meet's own Speakers picker
set to the Multi-Output Device - see gotcha above), and watch the transcript;
it's also being written live to `/tmp/voice-input/transcript-<timestamp>.txt`.

Drop `--engine whisper --language bg --diarize` (i.e. just `--device BlackHole`)
to test the faster Voxtral Realtime path on English or another supported language.

## Known bugs to revisit
- **Phrase repetition within a segment**: `StreamingDecoder`'s AlignAtt algorithm occasionally re-decodes and rewords part of a phrase between consecutive ~1s ticks, and the length-based token diff (`text_tokens[len(emitted):]`) doesn't catch that as a revision, so it gets emitted twice. Mitigated (not fixed) by resetting the decoder every `MAX_SEGMENT_SECONDS=20s` in `voxtral/realtime.py::run_whisper` — this fixed the far worse cross-30s-window duplication (see StreamingDecoder's silent sliding-window trim), but same-segment repeats still happen. A real fix needs longest-common-prefix diffing instead of length-based, or accepting occasional revision the way commercial live-caption UIs show provisional text being replaced.
- **Unconfirmed UI report**: user flagged "the UI has errors" but we couldn't inspect the actual browser tab (Chrome extension not connected in this environment - needs the Claude Chrome extension installed and logged into the same account). Backend was independently verified healthy at the time (page 200, `/events` SSE headers arrive immediately, valid JSON deltas). Revisit with Chrome extension access, or ask for a screenshot / DevTools console errors.

## Known gaps to revisit
- No native function-calling from local Voxtral — `mlx-audio`'s wrapper only returns plain transcribed text, so voice → Mac-control actions (`internal/actions`, not built yet) would need a separate intent layer.
- Voxtral TTS weights are CC-BY-NC (non-commercial use only), and don't cover Bulgarian either - a spoken-Bulgarian response would need a different TTS model.
- `--diarize` can only ever distinguish remote participants already mixed into Meet's audio output - it can never label this machine's own mic input, since the mic never flows through the BlackHole capture path at all. Full "who said what including me" would need a second, separate mic-capture path merged in.
