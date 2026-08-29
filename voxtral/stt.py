#!/usr/bin/env python3
"""Voxtral Mini 3B one-shot transcription primitive.

Push-to-talk counterpart to pkg/whisper: takes a recorded audio file and
prints the transcript, the same shape whisper-cli output takes today.
"""

import argparse
import json
import sys

DEFAULT_MODEL = "mlx-community/Voxtral-Mini-3B-2507-bf16"


def main():
    parser = argparse.ArgumentParser(description="Transcribe audio with Voxtral Mini 3B (MLX)")
    parser.add_argument("--audio", required=True, help="Path to audio file (wav)")
    parser.add_argument("--model", default=DEFAULT_MODEL, help="HF repo id or local path")
    parser.add_argument("--language", default="en", help="Language code")
    parser.add_argument("--max-tokens", type=int, default=1024)
    parser.add_argument("--output", help="Optional path to also write the transcript to")
    parser.add_argument(
        "--json", action="store_true", help='Print {"text": ...} instead of plain text'
    )
    args = parser.parse_args()

    from mlx_audio.stt.utils import load_model

    model = load_model(args.model)
    result = model.generate(
        audio=args.audio,
        language=args.language,
        max_tokens=args.max_tokens,
        temperature=0.0,
    )
    text = result.text.strip()

    if args.output:
        with open(args.output, "w") as f:
            f.write(text)

    if args.json:
        json.dump({"text": text}, sys.stdout)
    else:
        print(text)


if __name__ == "__main__":
    main()
