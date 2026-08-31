#!/bin/bash
# Claude Code Stop hook: speak a short snippet of Claude's last message when it
# finishes responding and hands control back to you.
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"
read_hook_input

# Everything below can be slow (LLM summarization is a multi-second local
# inference call), so it all runs backgrounded — the hook itself returns to
# Claude Code immediately regardless of which path is taken.
(
    # Skip the transcript parsing/summarization work entirely while muted —
    # speak.sh would silence it anyway, but there's no point paying for an
    # Ollama call whose result will never be heard.
    voice_is_muted && exit 0

    TRANSCRIPT=$(echo "$HOOK_INPUT" | jq -r '.transcript_path // empty' 2>/dev/null)

    if [ -z "$TRANSCRIPT" ] || [ ! -f "$TRANSCRIPT" ]; then
        speak_hook_message "Claude finished."
        exit 0
    fi

    # Each content block (text vs tool_use) tends to land as its own assistant
    # entry in the transcript, so just take the most recent text block in the
    # window rather than assuming they're grouped under one message.
    TEXT=$(tail -200 "$TRANSCRIPT" | jq -rs '
      [.[] | select(.type=="assistant") | (.message.content // [])[]? | select(.type=="text") | .text]
      | last // empty
    ' 2>/dev/null)

    if [ -z "$TEXT" ]; then
        speak_hook_message "Claude finished."
        exit 0
    fi

    # A max_chars <= 0 (set via the "No limit" toggle in the menu bar app) means unlimited —
    # "No limit" is a hard promise to never mid-sentence-cut, but with LLM summary on, a
    # genuinely long message still gets summarized rather than read verbatim in full.
    MAX_CHARS="${TTS_STOP_MAX_CHARS:-$(config_get_int stopMaxChars 600)}" # 600 is a last-resort fallback only; see voice-defaults.json
    LLM_SUMMARY_ON=$(config_get_bool llmSummary false)
    MODEL=$(config_get summaryModel)
    [ -z "$MODEL" ] && MODEL="qwen2.5:3b"

    # Markdown stripping, length-check, optional Ollama summarization, and
    # sentence-preserving truncation all happen in one voice_hooks_run call
    # (scripts/voice_hooks/stop.py) — see scripts/voice_hooks/ for the logic.
    SNIPPET=$(printf '%s' "$TEXT" | voice_hooks_run stop.py \
        --max-chars "$MAX_CHARS" \
        --llm-summary "$LLM_SUMMARY_ON" \
        --model "$MODEL" 2>/dev/null)

    if [ $? -ne 0 ]; then
        # voice_hooks CLI failed outright (unsynced venv, uv off PATH, etc.)
        # — speak the raw transcript text rather than silently say nothing.
        SNIPPET="$TEXT"
    fi

    if [ -z "$SNIPPET" ]; then
        speak_hook_message "Claude finished."
    else
        speak_hook_message "Claude finished: $SNIPPET"
    fi
) &
disown
exit 0
