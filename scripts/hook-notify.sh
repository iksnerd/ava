#!/bin/bash
# Claude Code Notification hook: speak the notification (e.g. "Claude needs your
# permission to use Bash", "Claude is waiting for your input").
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"
read_hook_input

# A max_chars <= 0 (set via the "No limit" toggle in the menu bar app) means unlimited.
MAX_CHARS="${TTS_NOTIFY_MAX_CHARS:-$(config_get_int notifyMaxChars 500)}" # 500 is a last-resort fallback only; see voice-defaults.json

MSG=$(echo "$HOOK_INPUT" | jq -r '.message // empty' 2>/dev/null)
if [ -z "$MSG" ]; then
    MSG="Claude needs your attention."
fi
if [ "$MAX_CHARS" -gt 0 ] && [ "${#MSG}" -gt "$MAX_CHARS" ]; then
    TRUNCATED=$(printf '%s' "$MSG" | voice_hooks_run notify.py --max-chars "$MAX_CHARS" 2>/dev/null)
    # Only replace MSG on success — a failed voice_hooks CLI (unsynced venv,
    # uv off PATH) should fall through to speaking the untruncated message,
    # never nothing.
    [ $? -eq 0 ] && [ -n "$TRUNCATED" ] && MSG="$TRUNCATED"
fi

speak_hook_message "$MSG"
exit 0
