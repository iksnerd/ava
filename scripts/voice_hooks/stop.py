#!/usr/bin/env python3
"""CLI entrypoint for hook-stop.sh: decides the spoken snippet for a
finished Claude response. Reads raw response text on stdin, already-resolved
config as flags (bash owns config resolution — this stays a pure function of
its inputs), prints the final snippet to stdout.
"""

import argparse
import sys

from ollama_client import ollama_summarize
from text import strip_markdown, truncate_keep_last_sentence


def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser()
    p.add_argument("--max-chars", type=int, required=True)
    # A string flag, not store_true/BooleanOptionalAction: bash's
    # config_get_bool already produces exactly "true"/"false", so this is a
    # direct passthrough with no translation layer at the call site.
    p.add_argument("--llm-summary", choices=("true", "false"), default="false")
    p.add_argument("--model", default="qwen2.5:3b")
    p.add_argument("--unlimited-summary-trigger-chars", type=int, default=800)
    p.add_argument("--unlimited-summary-budget-chars", type=int, default=600)
    p.add_argument("--ollama-url", default="http://127.0.0.1:11434")
    p.add_argument("--ollama-timeout", type=float, default=12.0)
    return p


def resolve_snippet(raw_text: str, args: argparse.Namespace) -> str:
    clean = strip_markdown(raw_text)
    if not clean:
        return ""

    unlimited = args.max_chars <= 0
    fits = (not unlimited and len(clean) <= args.max_chars) or (
        unlimited and len(clean) <= args.unlimited_summary_trigger_chars
    )
    if fits:
        return clean

    snippet = ""
    if args.llm_summary == "true":
        budget = args.unlimited_summary_budget_chars if unlimited else args.max_chars
        snippet = ollama_summarize(
            clean, budget, args.model, base_url=args.ollama_url, timeout=args.ollama_timeout
        )
    if not snippet:
        # Summarization off/failed but "No limit" is set — honor it: full
        # text, never a truncation.
        snippet = clean if unlimited else truncate_keep_last_sentence(clean, args.max_chars)
    return snippet


def main() -> int:
    args = build_parser().parse_args()
    print(resolve_snippet(sys.stdin.read(), args))
    return 0


if __name__ == "__main__":
    sys.exit(main())
