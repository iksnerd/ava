#!/usr/bin/env python3
"""CLI entrypoint for hook-notify.sh: truncates a notification message to fit
a length cap while keeping its true last sentence intact.
"""

import argparse
import sys

from text import truncate_keep_last_sentence


def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser()
    p.add_argument("--max-chars", type=int, required=True)
    return p


def resolve_message(raw_text: str, max_chars: int) -> str:
    msg = raw_text.strip()
    if max_chars <= 0 or len(msg) <= max_chars:
        return msg
    return truncate_keep_last_sentence(msg, max_chars)


def main() -> int:
    args = build_parser().parse_args()
    print(resolve_message(sys.stdin.read(), args.max_chars))
    return 0


if __name__ == "__main__":
    sys.exit(main())
