#!/bin/bash
# Immediately cancels any speech currently in flight from speak.sh — both
# audio already playing and synthesis still being generated — instead of
# waiting for it to finish naturally. Used by the menu bar app's "Stop
# Speaking" button.
#
# Usage: stop-speaking.sh
ACTIVITY_DIR="/tmp/claude-tts-active"

shopt -s nullglob
for marker in "$ACTIVITY_DIR"/*; do
    case "$marker" in
        *.synth.pid | *.play.pid | *.stopped) continue ;;
    esac
    [ -f "$marker" ] || continue

    # Tells speak.sh's own wait_synth that this was a deliberate stop, not a
    # failure, so it doesn't fall back to the next TTS option (e.g. `say`).
    touch "$marker.stopped" 2>/dev/null

    for pidfile in "$marker.synth.pid" "$marker.play.pid"; do
        [ -f "$pidfile" ] || continue
        pid=$(cat "$pidfile" 2>/dev/null)
        [ -n "$pid" ] && kill "$pid" 2>/dev/null
    done
done

exit 0
