#!/usr/bin/env python3
"""Realtime streaming transcription primitive: Voxtral Mini 4B Realtime or
multilingual Whisper (both via mlx-audio).

Voxtral Realtime is the low-latency default (<500ms) but only covers 13
languages (no Bulgarian). --engine whisper switches to mlx-audio's Whisper
STT wrapper, driven the same way but with ~1s chunked latency, for the
99+ languages the Voxtral models don't cover.

Listens on an input device (default mic, or any other input device such as
a BlackHole loopback capturing system/call audio) and prints one JSON object
per line to stdout as transcript deltas arrive, so a long-lived Go subprocess
can read it line-by-line. Runs until interrupted (Ctrl+C / SIGINT), then
flushes whatever was still pending before exiting.
"""
import argparse
import json
import sys
import time

import numpy as np
import sounddevice as sd

DEFAULT_VOXTRAL_MODEL = "mlx-community/Voxtral-Mini-4B-Realtime-2602-4bit"
DEFAULT_WHISPER_MODEL = "mlx-community/whisper-large-v3-turbo-asr-fp16"
DEFAULT_DIARIZE_MODEL = "mlx-community/diar_sortformer_4spk-v1-fp32"
SAMPLE_RATE = 16000


def emit(event, **fields):
    print(json.dumps({"event": event, **fields}), flush=True)


class HighPassFilter:
    """Streaming (state-carried) Butterworth high-pass filter, applied per
    chunk to cut low-frequency rumble/bass before it reaches STT/diarization
    or inflates the silence-RMS check. Carries filter state (zi) across
    chunks so there's no click/discontinuity at chunk boundaries."""

    def __init__(self, cutoff_hz, order=2, sample_rate=SAMPLE_RATE):
        from scipy.signal import butter, sosfilt_zi

        self.sos = butter(order, cutoff_hz, btype="highpass", fs=sample_rate, output="sos")
        self.zi = sosfilt_zi(self.sos) * 0.0

    def __call__(self, chunk):
        from scipy.signal import sosfilt

        if chunk.size == 0:
            return chunk
        filtered, self.zi = sosfilt(self.sos, chunk, zi=self.zi)
        return filtered.astype(np.float32)


def device_name(device):
    idx = device if device is not None else sd.default.device[0]
    return sd.query_devices(idx)["name"]


def list_devices():
    default_input = sd.default.device[0]
    for idx, dev in enumerate(sd.query_devices()):
        if dev["max_input_channels"] <= 0:
            continue
        marker = " (default)" if idx == default_input else ""
        print(f"[{idx}] {dev['name']} - {dev['max_input_channels']}ch @ {int(dev['default_samplerate'])}Hz{marker}")


def resolve_device(spec):
    """Resolve --device to a sounddevice index. Accepts an index or a
    case-insensitive substring of the device name (e.g. "BlackHole")."""
    if spec is None:
        return None
    try:
        return int(spec)
    except ValueError:
        pass

    spec_lower = spec.lower()
    devices = sd.query_devices()
    matches = [i for i, d in enumerate(devices) if d["max_input_channels"] > 0 and spec_lower in d["name"].lower()]

    if not matches:
        raise SystemExit(f"No input device matching {spec!r}. Run with --list-devices to see options.")
    if len(matches) > 1:
        names = ", ".join(f"[{i}] {devices[i]['name']}" for i in matches)
        raise SystemExit(f"Multiple input devices match {spec!r}: {names}. Use an index to disambiguate.")
    return matches[0]


def run_voxtral(model_name, device, transcription_delay_ms, highpass_hz=0.0):
    from mlx_audio.stt.utils import load_model

    model = load_model(model_name)
    session = model.create_streaming_session(transcription_delay_ms=transcription_delay_ms)
    hpf = HighPassFilter(highpass_hz) if highpass_hz > 0 else None

    def on_audio(indata, frames, time_info, status):
        samples = indata[:, 0].copy()
        if hpf is not None:
            samples = hpf(samples)
        session.feed(samples)

    stream = sd.InputStream(samplerate=SAMPLE_RATE, channels=1, dtype="float32", device=device, callback=on_audio)

    emit("ready", device=device_name(device), engine="voxtral")

    try:
        with stream:
            while not session.done:
                for delta in session.step(max_decode_tokens=16):
                    emit("delta", text=delta)
                time.sleep(0.01)
    except KeyboardInterrupt:
        pass
    finally:
        session.close()
        while not session.done:
            for delta in session.step(max_decode_tokens=16):
                emit("delta", text=delta)

    emit("done")


def run_whisper(model_name, device, language, chunk_duration, frame_threshold, diarize_model_name=None, highpass_hz=0.0):
    from mlx_audio.stt.models.whisper.audio import log_mel_spectrogram
    from mlx_audio.stt.models.whisper.streaming import StreamingConfig, StreamingDecoder
    from mlx_audio.stt.utils import load_model

    model = load_model(model_name)
    hpf = HighPassFilter(highpass_hz) if highpass_hz > 0 else None

    diar_model = None
    diar_state = None
    if diarize_model_name:
        from mlx_audio.vad import load as load_vad

        diar_model = load_vad(diarize_model_name)
        diar_state = diar_model.init_streaming_state()

    def new_decoder():
        # StreamingDecoder can't auto-detect language like the batch API does -
        # it defaults to English unless told otherwise, so pass --language
        # explicitly for anything else (e.g. "bg" for Bulgarian).
        return StreamingDecoder(model, StreamingConfig(frame_threshold=frame_threshold), language=language)

    decoder = new_decoder()
    segment_seconds = 0.0
    # StreamingDecoder accumulates mel across calls but silently trims to a
    # 30s sliding window once it grows past that - its emitted-token count
    # tracking doesn't survive that trim, so on a call running longer than
    # ~30s it starts re-emitting stale, reworded text. Finalize and start a
    # fresh decoder comfortably before that boundary instead of hitting it.
    MAX_SEGMENT_SECONDS = 20.0
    # StreamingDecoder skips Whisper's normal no-speech gating entirely - fed
    # pure silence, it happily hallucinates short filler tokens instead of
    # staying quiet. Skip decoding chunks that are essentially silent instead.
    # Kept low: BlackHole loopback audio runs much quieter than mic input
    # (observed ~0.002 RMS during real speech), so a threshold tuned for a
    # mic would silently drop real speech here.
    SILENCE_RMS_THRESHOLD = 0.0006

    buf = []

    def on_audio(indata, frames, time_info, status):
        buf.append(indata[:, 0].copy())

    stream = sd.InputStream(samplerate=SAMPLE_RATE, channels=1, dtype="float32", device=device, callback=on_audio)

    emit("ready", device=device_name(device), engine="whisper", language=language or "en", diarize=diar_model is not None)

    def flush(is_last):
        nonlocal buf, decoder, segment_seconds, diar_state
        if not buf and not is_last:
            return
        chunk = np.concatenate(buf) if buf else np.zeros(0, dtype=np.float32)
        buf = []
        if hpf is not None:
            chunk = hpf(chunk)

        is_silent = chunk.size == 0 or np.sqrt(np.mean(chunk.astype(np.float64) ** 2)) < SILENCE_RMS_THRESHOLD
        if is_silent and not is_last:
            return

        segment_seconds += len(chunk) / SAMPLE_RATE
        force_final = is_last or segment_seconds >= MAX_SEGMENT_SECONDS

        # Diarize this same chunk. Coarse (per-chunk, not per-word): whichever
        # speaker talks most within this ~chunk_duration slice tags any text
        # emitted alongside it. Only ever sees remote participants - BlackHole
        # captures Meet's mixed output, never this machine's own mic input.
        speaker = None
        if diar_model is not None and chunk.size > 0:
            for dresult in diar_model.generate_stream(chunk, state=diar_state, sample_rate=SAMPLE_RATE):
                diar_state = dresult.state
                if dresult.segments:
                    longest = max(dresult.segments, key=lambda s: s.end - s.start)
                    speaker = f"Speaker {longest.speaker}"

        mel = log_mel_spectrogram(chunk, n_mels=model.dims.n_mels)
        result = decoder.decode_chunk(mel, is_last=force_final)
        if result.text.strip():
            emit("delta", text=result.text + (" " if force_final else ""), speaker=speaker or "")

        if force_final and not is_last:
            decoder = new_decoder()
            segment_seconds = 0.0

    try:
        with stream:
            while True:
                time.sleep(chunk_duration)
                flush(is_last=False)
    except KeyboardInterrupt:
        pass
    finally:
        flush(is_last=True)

    emit("done")


def main():
    parser = argparse.ArgumentParser(description="Realtime transcription via Voxtral Realtime or Whisper (MLX)")
    parser.add_argument(
        "--engine",
        choices=["voxtral", "whisper"],
        default="voxtral",
        help="'voxtral' = Voxtral Mini 4B Realtime, <500ms latency, 13 languages. "
        "'whisper' = multilingual Whisper (99+ languages incl. Bulgarian), ~1s latency.",
    )
    parser.add_argument("--model", default=None, help="HF repo id or local path (default depends on --engine)")
    parser.add_argument(
        "--language",
        default=None,
        help="Language code, e.g. 'bg' for Bulgarian. Whisper engine only; defaults to English if omitted.",
    )
    parser.add_argument("--chunk-duration", type=float, default=1.0, help="Whisper engine: seconds of audio per decode step")
    parser.add_argument(
        "--frame-threshold",
        type=int,
        default=25,
        help="Whisper engine: AlignAtt threshold, lower = faster/less accurate (default 25, ~0.5s lookahead)",
    )
    parser.add_argument(
        "--transcription-delay-ms",
        type=int,
        default=None,
        help="Voxtral engine: override the model's default emit delay (lower = faster, less accurate)",
    )
    parser.add_argument(
        "--diarize",
        action="store_true",
        help="Whisper engine only: tag each delta with a speaker label (e.g. 'Speaker 0') via Sortformer "
        "diarization. Only distinguishes remote participants already mixed into Meet's audio output - "
        "can never label your own mic input, since that never flows through this pipeline.",
    )
    parser.add_argument("--diarize-model", default=None, help="Override the diarization model HF repo id")
    parser.add_argument(
        "--highpass-hz",
        type=float,
        default=80.0,
        help="High-pass filter cutoff in Hz to cut low-frequency rumble/bass before STT/diarization "
        "(0 disables). Default 80Hz, below typical voice fundamental frequency.",
    )
    parser.add_argument(
        "--device",
        default=None,
        help="Input device name (substring, e.g. 'BlackHole') or index. Default: system default mic.",
    )
    parser.add_argument("--list-devices", action="store_true", help="List available input devices and exit")
    args = parser.parse_args()

    if args.list_devices:
        list_devices()
        return

    device = resolve_device(args.device)

    if args.engine == "whisper":
        diarize_model = (args.diarize_model or DEFAULT_DIARIZE_MODEL) if args.diarize else None
        run_whisper(
            args.model or DEFAULT_WHISPER_MODEL,
            device,
            args.language,
            args.chunk_duration,
            args.frame_threshold,
            diarize_model,
            args.highpass_hz,
        )
    else:
        if args.diarize:
            print("--diarize is only supported with --engine whisper; ignoring.", file=sys.stderr)
        run_voxtral(args.model or DEFAULT_VOXTRAL_MODEL, device, args.transcription_delay_ms, args.highpass_hz)


if __name__ == "__main__":
    main()
