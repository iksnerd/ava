#!/bin/bash
# Speak text using the local Voxtral/Kokoro MLX server (scripts/voxtral-server.sh),
# falling back to macOS's built-in `say` if the server can't be reached.
#
# Usage: speak.sh "text to speak" [voice]
#
# Designed to be called from Claude Code hooks: always returns fast (backgrounds
# the actual synthesis+playback) so it never stalls the hook that invoked it.
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

SERVER="http://127.0.0.1:8765"
TEXT="$1"
VOICE="${2:-$(config_get voice)}"
SPEED="${TTS_SPEED:-$(config_get speed)}"       # Kokoro speed multiplier
VOLUME="${TTS_VOLUME:-$(config_get volume)}"    # afplay volume, 0.0-1.0+
SAY_RATE="${TTS_SAY_RATE:-$(config_get sayRate)}" # words/min for the `say` fallback
PLAYBACK_LOCK="/tmp/claude-tts-playback.lock"
PLAYBACK_TIMEOUT_SEC=30

if [ -z "$TEXT" ]; then
    echo "Usage: $0 \"text to speak\" [voice]" >&2
    exit 1
fi

# Multiple Claude Code sessions can call this at once. The server already
# serializes /speak generation (single-worker, synchronous), but playback
# doesn't unless we lock it too — so wrap afplay/say in a cross-process
# mutex (fcntl advisory lock) so sessions queue instead of talking over each
# other. A hard timeout on the *held* command keeps one wedged player from
# permanently blocking every other session's audio.
play_locked() {
    python3 -c "
import fcntl, subprocess, sys
lock_path, timeout, cmd = sys.argv[1], float(sys.argv[2]), sys.argv[3:]
with open(lock_path, 'w') as f:
    fcntl.flock(f, fcntl.LOCK_EX)
    try:
        subprocess.run(cmd, timeout=timeout)
    except subprocess.TimeoutExpired:
        pass
" "$PLAYBACK_LOCK" "$PLAYBACK_TIMEOUT_SEC" "$@"
}

speak_with_server() {
    local out
    out="$(mktemp -t claude-tts).wav"
    local payload
    payload=$(python3 -c 'import json,sys; print(json.dumps({"text": sys.argv[1], "voice": sys.argv[2], "speed": float(sys.argv[3])}))' "$TEXT" "$VOICE" "$SPEED")
    if curl -s -f -m 20 -X POST "$SERVER/speak" \
        -H "Content-Type: application/json" \
        -d "$payload" \
        -o "$out"; then
        play_locked afplay -v "$VOLUME" "$out"
        rm -f "$out"
        return 0
    fi
    rm -f "$out"
    return 1
}

# `say` has no volume flag, so render to a file and play it through afplay
# too — keeps the volume slider consistent even when the server's down.
speak_with_say_fallback() {
    local aiff
    aiff="$(mktemp -t claude-tts-say).aiff"
    if say -r "$SAY_RATE" -o "$aiff" "$TEXT"; then
        play_locked afplay -v "$VOLUME" "$aiff"
    fi
    rm -f "$aiff"
}

(
    if curl -s -f -m 2 "$SERVER/health" >/dev/null 2>&1; then
        speak_with_server || speak_with_say_fallback
    elif bash "$SCRIPT_DIR/voxtral-server.sh" start >/tmp/claude-tts-server-start.log 2>&1; then
        speak_with_server || speak_with_say_fallback
    else
        speak_with_say_fallback
    fi
) &
disown
exit 0
