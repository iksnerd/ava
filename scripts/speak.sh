#!/bin/bash
# Speak text using the local Kokoro TTS server (scripts/mlx-engine-server.sh),
# falling back to macOS's built-in `say` if the server can't be reached.
#
# Usage: speak.sh "text to speak" [voice]
#
# Designed to be called from Claude Code hooks: always returns fast (backgrounds
# the actual synthesis+playback) so it never stalls the hook that invoked it.
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"
# Generated from internal/protocol/protocol.json — the values every runtime
# has to agree on. Never edit protocol.sh; run `make generate-protocol`.
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/protocol.sh"
# The protocol's test overrides, honoured here as in Go (see protocol.json).
ACTIVITY_DIR="${!ACTIVITY_DIR_ENV:-$ACTIVITY_DIR}"
PLAYBACK_LOCK="${!PLAYBACK_LOCK_ENV:-$PLAYBACK_LOCK}"

SERVER="$ENGINE_URL"
TEXT="$1"
# VOICE, SPEED (Kokoro multiplier), VOLUME (afplay, 0.0-1.0+) and SAY_RATE
# (words/min for the `say` fallback), with TTS_* env overrides. One function
# in lib.sh so the bash/Go contract test runs exactly what speaks.
resolve_speak_settings "${2:-}"
PLAYBACK_TIMEOUT_SEC=600 # long enough for a full article read aloud at once

if [ -z "$TEXT" ]; then
    echo "Usage: $0 \"text to speak\" [voice]" >&2
    exit 1
fi

# Global mute (menu bar app) — the one choke point every caller goes
# through (hooks, Read Aloud, Test/Preview), so muting is absolute rather
# than something each caller has to remember to check.
if voice_is_muted; then
    exit 0
fi

# One marker file per in-flight speak, present for the whole synth+playback
# duration below (removed on any exit path via the trap) — polled by
# AvaMenuBar's SpeechActivityMonitor to show a "speaking" indicator,
# and used by stop-speaking.sh to find what to cancel. PID-named so
# concurrent speaks (multiple Claude Code sessions, or a hook overlapping a
# manual Read Aloud) don't clobber each other's marker.
# ACTIVITY_DIR comes from protocol.sh, sourced above.

# Multiple Claude Code sessions can call this at once. The server already
# serializes /speak generation (single-worker, synchronous), but playback
# doesn't unless we lock it too — so play_locked.py wraps afplay in the same
# flock the Go speaker takes, and follows the same stop protocol (see its
# docstring).
play_locked() {
    python3 "$SCRIPT_DIR/play_locked.py" "$PLAYBACK_LOCK" "$PLAYBACK_TIMEOUT_SEC" \
        "${ACTIVITY_MARKER}$PLAY_PID_SUFFIX" "${ACTIVITY_MARKER}$STOPPED_SUFFIX" "$@"
}

# Runs $1 (a synthesis command already backgrounded by the caller via `&`)
# and waits on it while tracking its PID for stop-speaking.sh. Sets
# STOPPED=1 if stop-speaking.sh cancelled it (vs. it just failing on its
# own), so callers know not to fall back to the next TTS option.
wait_synth() {
    local pid="$1"
    echo "$pid" > "${ACTIVITY_MARKER}$SYNTH_PID_SUFFIX"
    wait "$pid"
    local status=$?
    rm -f "${ACTIVITY_MARKER}$SYNTH_PID_SUFFIX"
    if [ -e "${ACTIVITY_MARKER}$STOPPED_SUFFIX" ]; then
        STOPPED=1
        return 1
    fi
    return "$status"
}

speak_with_server() {
    local out="$SPEAK_TMP/speech.wav"
    local payload
    payload=$(python3 -c 'import json,sys; print(json.dumps({"text": sys.argv[1], "voice": sys.argv[2], "speed": float(sys.argv[3])}))' "$TEXT" "$VOICE" "$SPEED")
    curl -s -f -m 300 -X POST "$SERVER/speak" \
        -H "Content-Type: application/json" \
        -d "$payload" \
        -o "$out" &
    if wait_synth "$!"; then
        play_locked afplay -v "$VOLUME" "$out"
        return 0
    fi
    return 1
}

# `say` has no volume flag, so render to a file and play it through afplay
# too — keeps the volume slider consistent even when the server's down.
speak_with_say_fallback() {
    local aiff="$SPEAK_TMP/speech.aiff"
    say -r "$SAY_RATE" -o "$aiff" "$TEXT" &
    if wait_synth "$!"; then
        play_locked afplay -v "$VOLUME" "$aiff"
    fi
}

(
    mkdir -p "$ACTIVITY_DIR" 2>/dev/null
    ACTIVITY_MARKER="$ACTIVITY_DIR/$$"
    touch "$ACTIVITY_MARKER" 2>/dev/null
    # One directory for this speak's audio, removed on every exit path. It
    # used to be mktemp's file with .wav or .aiff appended, which left the
    # unsuffixed file behind on every speak; and `mktemp -t` ignores $TMPDIR
    # on macOS, where the leftovers piled up out of sight.
    SPEAK_TMP="$(mktemp -d "${TMPDIR:-/tmp}/ava-tts.XXXXXX")"
    trap 'rm -rf "$SPEAK_TMP"; rm -f "$ACTIVITY_MARKER" "${ACTIVITY_MARKER}$SYNTH_PID_SUFFIX" "${ACTIVITY_MARKER}$PLAY_PID_SUFFIX" "${ACTIVITY_MARKER}$STOPPED_SUFFIX"' EXIT
    STOPPED=0

    if curl -s -f -m 2 "$SERVER/health" >/dev/null 2>&1; then
        speak_with_server || { [ "$STOPPED" = 1 ] || speak_with_say_fallback; }
    elif voice_engine_autostart_enabled && bash "$SCRIPT_DIR/mlx-engine-server.sh" start >/tmp/ava-tts-server-start.log 2>&1; then
        speak_with_server || { [ "$STOPPED" = 1 ] || speak_with_say_fallback; }
    else
        speak_with_say_fallback
    fi
) &
disown
exit 0
