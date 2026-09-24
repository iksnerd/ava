#!/bin/bash
# Immediately cancels any speech currently in flight — both
# audio already playing and synthesis still being generated — instead of
# waiting for it to finish naturally. Used by the menu bar app's "Stop
# Speaking" button.
#
# Usage: stop-speaking.sh
# Generated from internal/protocol/protocol.json; supplies ACTIVITY_DIR and
# the sidecar suffixes. Never edit protocol.sh; run `make generate-protocol`.
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/protocol.sh"
ACTIVITY_DIR="${!ACTIVITY_DIR_ENV:-$ACTIVITY_DIR}"

shopt -s nullglob
for marker in "$ACTIVITY_DIR"/*; do
    case "$marker" in
        *"$SYNTH_PID_SUFFIX" | *"$PLAY_PID_SUFFIX" | *"$STOPPED_SUFFIX") continue ;;
    esac
    [ -f "$marker" ] || continue

    # Tells the speak this was a deliberate stop, not a failure, so it
    # neither plays nor falls back to the next TTS option (e.g. `say`).
    touch "${marker}$STOPPED_SUFFIX" 2>/dev/null

    for pidfile in "${marker}$SYNTH_PID_SUFFIX" "${marker}$PLAY_PID_SUFFIX"; do
        [ -f "$pidfile" ] || continue
        pid=$(cat "$pidfile" 2>/dev/null)
        [ -n "$pid" ] && kill "$pid" 2>/dev/null
    done
done

exit 0
