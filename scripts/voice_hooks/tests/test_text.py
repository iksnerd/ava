import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from text import strip_markdown, truncate_keep_last_sentence  # noqa: E402


def test_short_text_unchanged():
    text = "Short message."
    assert truncate_keep_last_sentence(text, 500) == text


def test_long_text_keeps_head_and_last_sentence():
    text = (
        "This is the first sentence of a fairly long response that goes on and on "
        "with lots of extra padding words here to make it longer than the budget "
        "allows for sure. This is the middle sentence which adds a bit more detail "
        "and context to the situation at hand. This is the final sentence that "
        "matters most and should always be heard."
    )
    result = truncate_keep_last_sentence(text, 200)
    assert result.endswith(
        "This is the final sentence that matters most and should always be heard."
    )
    assert result.startswith("This is the first sentence")


def test_tiny_budget_returns_last_sentence_only():
    text = (
        "This is the first sentence of a fairly long response that goes on and on. "
        "This is the final sentence that matters most."
    )
    result = truncate_keep_last_sentence(text, 60)
    assert result == "This is the final sentence that matters most."


def test_abbreviation_not_treated_as_sentence_boundary():
    from text import _SEGMENTER

    text = (
        "We support many formats, e.g. JSON and YAML, and Dr. Smith reviewed the "
        "design. The final sentence explains why this matters for the rollout plan "
        "going forward into next quarter."
    )
    # A naive `.!?`-regex split would treat "e.g." and "Dr." as sentence ends,
    # fragmenting the first sentence into three. pysbd must keep it whole.
    sentences = [s.strip() for s in _SEGMENTER.segment(text) if s.strip()]
    assert len(sentences) == 2
    assert sentences[0] == (
        "We support many formats, e.g. JSON and YAML, and Dr. Smith reviewed the design."
    )

    result = truncate_keep_last_sentence(text, 90)
    assert result.endswith(
        "The final sentence explains why this matters for the rollout plan going forward into next quarter."
    )


def test_no_sentence_boundaries_speaks_whole_text():
    # No punctuation means pysbd treats the whole string as one sentence —
    # per the "last sentence alone eats the budget, speak it whole" rule,
    # the full text comes back rather than being cut mid-word.
    text = "a" * 1000
    result = truncate_keep_last_sentence(text, 50)
    assert result == text


def test_strip_markdown_removes_code_spans_and_noise():
    text = "Run `make build` then check **bold** and # headers > quotes"
    result = strip_markdown(text)
    assert "`" not in result
    assert "*" not in result
    assert "#" not in result
    assert ">" not in result
