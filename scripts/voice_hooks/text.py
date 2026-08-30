"""Markdown stripping and sentence-boundary-aware truncation for voice hooks.

Pure functions only — callers resolve config (max_chars, etc.) and pass it in.
"""

import re

import pysbd

_CODE_SPAN_RE = re.compile(r"`{1,3}[^`]*`{1,3}")
_MD_NOISE_RE = re.compile(r"[*_#>`]")

# Regex-table construction happens once here, not per call — this module is
# re-imported fresh on every hook firing (no long-lived process).
_SEGMENTER = pysbd.Segmenter(language="en", clean=False)


def strip_markdown(text: str) -> str:
    text = text.strip()
    text = _CODE_SPAN_RE.sub(" ", text)
    text = _MD_NOISE_RE.sub("", text)
    return " ".join(text.split())


def truncate_keep_last_sentence(text: str, max_chars: int) -> str:
    """Front-truncate `text` to fit `max_chars`, always keeping the text's
    true last sentence intact — a spoken snippet that silently drops its own
    conclusion is worse than one that's a little long or cuts its lead-in.

    Uses pysbd for real sentence segmentation (not a naive `.!?` regex, which
    mis-splits on abbreviations like "e.g." or "Dr.") to build both the head
    and the last sentence from the same pass.
    """
    text = text.strip()
    if len(text) <= max_chars:
        return text

    sentences = [s.strip() for s in _SEGMENTER.segment(text) if s.strip()]
    if not sentences:
        return text[:max_chars].rstrip() + "..."

    last = sentences[-1]
    budget = max_chars - len(last) - 5

    if budget <= max_chars // 3:
        # Last sentence alone eats the whole budget — just speak it, even if
        # that means exceeding max_chars slightly. A mid-sentence cut is worse.
        return last

    head_sentences: list[str] = []
    used = 0
    for s in sentences[:-1]:
        if used + len(s) + 1 > budget:
            break
        head_sentences.append(s)
        used += len(s) + 1
    head = " ".join(head_sentences).rstrip()

    if not head:
        head = text[:budget].rstrip() + "..."
    if head.endswith(last):
        return head
    return f"{head} ... {last}"
