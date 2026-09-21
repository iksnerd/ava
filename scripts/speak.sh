#!/bin/bash
# Speak text using the local Voxtral/Kokoro MLX server (scripts/mlx-engine-server.sh),
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
SPEED=$(config_get_float speed 1.3)             # Kokoro speed multiplier
VOLUME=$(config_get_float volume 1.0)           # afplay volume, 0.0-1.0+
# Env overrides go through the same check as the config file, so a typo'd
# TTS_SPEED falls back instead of reaching float() — matching what
# internal/voiceconfig does with the same variable.
[ -n "$TTS_SPEED" ] && SPEED=$(as_float "$TTS_SPEED" "$SPEED")
[ -n "$TTS_VOLUME" ] && VOLUME=$(as_float "$TTS_VOLUME" "$VOLUME")
SAY_RATE="${TTS_SAY_RATE:-$(config_get sayRate)}" # words/min for the `say` fallback
PLAYBACK_LOCK="/tmp/claude-tts-playback.lock"
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
# ClaudeVoiceMenuBar's SpeechActivityMonitor to show a "speaking" indicator,
# and used by stop-speaking.sh to find what to cancel. PID-named so
# concurrent speaks (multiple Claude Code sessions, or a hook overlapping a
# manual Read Aloud) don't clobber each other's marker.
ACTIVITY_DIR="/tmp/claude-tts-active"

# Multiple Claude Code sessions can call this at once. The server already
# serializes /speak generation (single-worker, synchronous), but playback
# doesn't unless we lock it too — so wrap afplay/say in a cross-process
# mutex (fcntl advisory lock) so sessions queue instead of talking over each
# other. A hard timeout on the *held* command keeps one wedged player from
# permanently blocking every other session's audio. Runs the player as its
# own tracked subprocess (rather than exec'ing bash's own PID over cmd) so
# stop-speaking.sh has a real PID to kill instead of only the lock-holding
# python wrapper.
play_locked() {
    python3 -c "
import fcntl, subprocess, sys, os
lock_path, timeout, pidfile, cmd = sys.argv[1], float(sys.argv[2]), sys.argv[3], sys.argv[4:]
# Written *before* acquiring the lock too (as this process's own pid), so a
# speak still queued behind another one's playback can still be killed by
# stop-speaking.sh instead of only ones already actually playing.
with open(pidfile, 'w') as pf:
    pf.write(str(os.getpid()))
with open(lock_path, 'w') as f:
    fcntl.flock(f, fcntl.LOCK_EX)
    proc = subprocess.Popen(cmd)
    with open(pidfile, 'w') as pf:
        pf.write(str(proc.pid))
    try:
        proc.wait(timeout=timeout)
    except subprocess.TimeoutExpired:
        proc.kill()
    finally:
        try:
            os.remove(pidfile)
        except OSError:
            pass
" "$PLAYBACK_LOCK" "$PLAYBACK_TIMEOUT_SEC" "$ACTIVITY_MARKER.play.pid" "$@"
}

# Runs $1 (a synthesis command already backgrounded by the caller via `&`)
# and waits on it while tracking its PID for stop-speaking.sh. Sets
# STOPPED=1 if stop-speaking.sh cancelled it (vs. it just failing on its
# own), so callers know not to fall back to the next TTS option.
wait_synth() {
    local pid="$1"
    echo "$pid" > "$ACTIVITY_MARKER.synth.pid"
    wait "$pid"
    local status=$?
    rm -f "$ACTIVITY_MARKER.synth.pid"
    if [ -e "$ACTIVITY_MARKER.stopped" ]; then
        STOPPED=1
        return 1
    fi
    return "$status"
}

speak_with_server() {
    local out
    out="$(mktemp -t claude-tts).wav"
    local payload
    payload=$(python3 -c 'import json,sys; print(json.dumps({"text": sys.argv[1], "voice": sys.argv[2], "speed": float(sys.argv[3])}))' "$TEXT" "$VOICE" "$SPEED")
    curl -s -f -m 300 -X POST "$SERVER/speak" \
        -H "Content-Type: application/json" \
        -d "$payload" \
        -o "$out" &
    if wait_synth "$!"; then
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
    say -r "$SAY_RATE" -o "$aiff" "$TEXT" &
    if wait_synth "$!"; then
        play_locked afplay -v "$VOLUME" "$aiff"
    fi
    rm -f "$aiff"
}

(
    mkdir -p "$ACTIVITY_DIR" 2>/dev/null
    ACTIVITY_MARKER="$ACTIVITY_DIR/$$"
    touch "$ACTIVITY_MARKER" 2>/dev/null
    trap 'rm -f "$ACTIVITY_MARKER" "$ACTIVITY_MARKER.synth.pid" "$ACTIVITY_MARKER.play.pid" "$ACTIVITY_MARKER.stopped"' EXIT
    STOPPED=0

    if curl -s -f -m 2 "$SERVER/health" >/dev/null 2>&1; then
        speak_with_server || { [ "$STOPPED" = 1 ] || speak_with_say_fallback; }
    elif voice_engine_autostart_enabled && bash "$SCRIPT_DIR/mlx-engine-server.sh" start >/tmp/claude-tts-server-start.log 2>&1; then
        speak_with_server || { [ "$STOPPED" = 1 ] || speak_with_say_fallback; }
    else
        speak_with_say_fallback
    fi
) &
disown
exit 0
