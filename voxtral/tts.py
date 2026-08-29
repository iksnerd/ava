#!/usr/bin/env python3
"""Voxtral 4B TTS primitive.

Synthesizes text to a wav file. Playback is left to the caller (afplay on
macOS, same as internal/clipboard.PlaySound already uses for sound effects)
rather than bundled here, to keep this a pure text-to-wav primitive.
"""

import argparse

DEFAULT_MODEL = "mlx-community/Voxtral-4B-TTS-2603-mlx-bf16"
DEFAULT_VOICE = "casual_male"


def main():
    parser = argparse.ArgumentParser(description="Synthesize speech with Voxtral TTS (MLX)")
    parser.add_argument("--text", required=True, help="Text to speak")
    parser.add_argument(
        "--voice", default=DEFAULT_VOICE, help="Voice preset, e.g. casual_male, cheerful_female"
    )
    parser.add_argument("--model", default=DEFAULT_MODEL, help="HF repo id or local path")
    parser.add_argument("--output", required=True, help="Path to write the synthesized wav file")
    args = parser.parse_args()

    import numpy as np
    from mlx_audio.audio_io import write as audio_write
    from mlx_audio.tts.utils import load

    model = load(args.model)

    # Voxtral TTS can yield multiple segments for longer text; concatenate
    # rather than taking the first one so long responses aren't truncated.
    segments = []
    sample_rate = None
    for result in model.generate(text=args.text, voice=args.voice):
        segments.append(np.array(result.audio))
        sample_rate = result.sample_rate

    audio = segments[0] if len(segments) == 1 else np.concatenate(segments)
    audio_write(args.output, audio, sample_rate)


if __name__ == "__main__":
    main()
