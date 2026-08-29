#!/bin/bash
# Claude Code Stop hook: speak a short snippet of Claude's last message when it
# finishes responding and hands control back to you.
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"
read_hook_input

# Everything below can be slow (LLM summarization is a multi-second local
# inference call), so it all runs backgrounded — the hook itself returns to
# Claude Code immediately regardless of which path is taken.
(
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

    CLEAN=$(printf '%s' "$TEXT" | python3 -c "
import sys, re
t = sys.stdin.read().strip()
t = re.sub(r'\`{1,3}[^\`]*\`{1,3}', ' ', t)   # strip inline/code spans
t = re.sub(r'[*_#>\`]', '', t)                # strip markdown noise
print(' '.join(t.split()))
")

    # A max_chars <= 0 (set via the "No limit" toggle in the menu bar app) means unlimited —
    # "No limit" is a hard promise to never mid-sentence-cut, but with LLM summary on, a
    # genuinely long message still gets summarized rather than read verbatim in full.
    MAX_CHARS="${TTS_STOP_MAX_CHARS:-$(config_get_int stopMaxChars 600)}" # 600 is a last-resort fallback only; see voice-defaults.json
    UNLIMITED_SUMMARY_TRIGGER_CHARS=800  # summarize past this even under "No limit"
    UNLIMITED_SUMMARY_BUDGET_CHARS=600   # word-budget basis for that summary (no MAX_CHARS to derive one from)

    LLM_SUMMARY_ON=$(config_get_bool llmSummary false)
    UNLIMITED=$([ "$MAX_CHARS" -le 0 ] && echo true || echo false)

    if { [ "$UNLIMITED" = "false" ] && [ "${#CLEAN}" -le "$MAX_CHARS" ]; } \
        || { [ "$UNLIMITED" = "true" ] && [ "${#CLEAN}" -le "$UNLIMITED_SUMMARY_TRIGGER_CHARS" ]; }; then
        SNIPPET="$CLEAN"
    else
        SNIPPET=""
        # Opt-in (off by default) — the "Summarize with local LLM" toggle in
        # the menu bar app. When on, replace mid-sentence truncation with an
        # actual summary from the local Ollama daemon.
        if [ "$LLM_SUMMARY_ON" = "true" ]; then
            budget="$MAX_CHARS"
            [ "$UNLIMITED" = "true" ] && budget="$UNLIMITED_SUMMARY_BUDGET_CHARS"
            SNIPPET=$(ollama_summarize "$CLEAN" "$budget")
        fi
        if [ -z "$SNIPPET" ]; then
            if [ "$UNLIMITED" = "true" ]; then
                # Summarization off/failed but "No limit" is set — honor it: full text, never cut.
                SNIPPET="$CLEAN"
            else
                SNIPPET=$(printf '%s' "$CLEAN" | python3 -c "
import sys
max_chars = int(sys.argv[1])
t = sys.stdin.read()
t = t[:max_chars]
cut = max(t.rfind('. '), t.rfind('! '), t.rfind('? '))
print(t[:cut+1] if cut > max_chars // 3 else t + '...')
" "$MAX_CHARS")
            fi
        fi
    fi

    if [ -z "$SNIPPET" ]; then
        speak_hook_message "Claude finished."
    else
        speak_hook_message "Claude finished: $SNIPPET"
    fi
) &
disown
exit 0
