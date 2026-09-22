#!/bin/bash
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

# engine_root prints the directory holding a usable mlx-engine/ (one whose venv
# exists), or fails. A checkout and the bundle `ava setup` installs both keep
# it beside scripts/. The copy of scripts/ bundled inside Ava.app does not:
# there ROOT_DIR is the app's Resources/, so the menu bar's Start button
# looked for Resources/mlx-engine, failed, and waited two minutes for a server
# that had never started. build-app.sh records the checkout it was built from
# in Resources/engine-root; the installed bundle is the last resort.
engine_root() {
    local recorded=""
    [ -f "$ROOT_DIR/engine-root" ] && recorded="$(cat "$ROOT_DIR/engine-root")"
    local installed="${AVA_ENGINE_DIR:-${LOCAL_WHISPER_ENGINE_DIR:-$HOME/Library/Application Support/ava/engine}}"
    local d
    for d in "$ROOT_DIR" "$recorded" "$installed"; do
        if [ -n "$d" ] && [ -x "$d/mlx-engine/.venv/bin/uvicorn" ]; then
            echo "$d"
            return 0
        fi
    done
    return 1
}
# Generated from internal/protocol/protocol.json; supplies ENGINE_PID_FILE
# and ENGINE_URL. Never edit protocol.sh; run `make generate-protocol`.
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/protocol.sh"
PID_FILE="$ENGINE_PID_FILE"
LOG_FILE="/tmp/mlx-engine-server.log"
START_LOCKDIR="/tmp/mlx-engine-server-start.lockdir"
START_LOCK_STALE_SEC=30

case "$1" in
    start)
        # An explicit start (menu bar Start, or `ava engine
        # start`) re-arms speak.sh's on-demand auto-start for future hook
        # firings — see voice_engine_autostart_enabled in lib.sh. Set
        # unconditionally, before any of this case's early exits, since
        # every one of them means "the server is/will be up" in some form.
        config_set_bool engineAutoStart true

        # Multiple Claude Code sessions can all find the server down at once
        # and race to start it — this lock ensures only one actually does,
        # while the rest wait for it to finish instead of double-spawning or
        # clobbering the PID file. See server_lock_acquire in lib.sh.
        if ! server_lock_acquire "$START_LOCKDIR" "$START_LOCK_STALE_SEC"; then
            echo "❌ Timed out waiting for another 'start' to finish."
            exit 1
        fi
        trap 'server_lock_release "$START_LOCKDIR"' EXIT

        if server_pidfile_alive "$PID_FILE"; then
            echo "✅ mlx-engine server is already running (PID: $(cat "$PID_FILE"))"
            exit 0
        fi
        if curl -s -f $ENGINE_URL/health > /dev/null 2>&1; then
            echo "✅ Server already responding on $ENGINE_URL (started outside this script; not touching its PID)."
            exit 0
        fi

        if ! DIR="$(engine_root)"; then
            echo "❌ No mlx-engine to start: none beside these scripts ($ROOT_DIR),"
            echo "   in the checkout this app was built from, or installed by \`ava setup\`."
            echo "   Run \`ava setup\`, or \`make setup\` in a checkout."
            exit 1
        fi

        echo "🚀 Starting the Kokoro TTS server from $DIR/mlx-engine..."
        cd "$DIR/mlx-engine" || exit 1
        uv sync -q
        # Exec the venv's own uvicorn directly instead of `uv run uvicorn` —
        # `uv run` can fork a child rather than exec into it, in which case
        # $! captures the wrapper's PID, not the real server, and killing it
        # later leaves an orphaned uvicorn process still bound to the port.
        # MLX_ENGINE_PID_FILE tells the server which file to clean up when it
        # shuts itself down on idle. It used to hardcode a different path than
        # this script writes, so a self-exit left the real pid file behind.
        # (It was never actually passed: a comment sat after the line
        # continuation, turning the assignment into a plain shell variable.
        # Harmless only because the server's default is the same path.)
        # Host and port are split out of the generated ENGINE_URL, so this
        # cannot bind somewhere the clients are not looking.
        engine_hostport="${ENGINE_URL#*//}"
        MLX_ENGINE_PID_FILE="$PID_FILE" nohup .venv/bin/uvicorn server:app \
            --host "${engine_hostport%%:*}" --port "${engine_hostport##*:}" \
            > "$LOG_FILE" 2>&1 &
        PID=$!
        echo $PID > "$PID_FILE"
        echo "✅ Server started with PID $PID. Logs at $LOG_FILE"

        echo "⏳ Waiting for server process to come up (models load lazily per-endpoint)..."
        for i in {1..120}; do
            if curl -s -f $ENGINE_URL/health > /dev/null; then
                echo "🎉 Server is up and ready!"
                exit 0
            fi
            # A server that died at startup will not come up by waiting; say
            # so now rather than after the full two minutes.
            if ! kill -0 "$PID" 2>/dev/null; then
                rm -f "$PID_FILE"
                echo "❌ The server exited during startup. Last lines of $LOG_FILE:"
                tail -5 "$LOG_FILE" | sed 's/^/   /'
                exit 1
            fi
            sleep 1
        done
        echo "❌ Server failed to start in time. Check $LOG_FILE"
        exit 1
        ;;
    stop)
        # A deliberate stop means "keep it off" — disarm speak.sh's
        # on-demand auto-start so the next Notification/Stop hook doesn't
        # silently bring the server right back up. See
        # voice_engine_autostart_enabled in lib.sh.
        #
        # --keep-autostart opts out, for the case this flag exists to serve:
        # something started the engine to check on it and wants to put the
        # machine back as it found it. Without it the only way to undo a
        # throwaway stop is to start the server again, which is the opposite
        # of tidying up. Two sessions have now tripped over that.
        if [ "${2:-}" = "--keep-autostart" ]; then
            keep_autostart=true
        elif [ -n "${2:-}" ]; then
            echo "❌ Unknown option for stop: $2 (only --keep-autostart)" >&2
            exit 1
        else
            keep_autostart=false
            config_set_bool engineAutoStart false
        fi

        if server_pidfile_alive "$PID_FILE"; then
            PID=$(cat "$PID_FILE")
            echo "🛑 Stopping mlx-engine server (PID: $PID)..."
            kill -9 "$PID"
        else
            echo "⚠️ Server is not running (per PID file)."
        fi
        rm -f "$PID_FILE"
        # `uv run` may fork rather than exec, leaving the actual uvicorn
        # process alive as an orphan even after the tracked PID is killed —
        # sweep for it explicitly so `stop` can't leave a stale server behind.
        if pkill -f "uvicorn server:app" 2>/dev/null; then
            echo "✅ Server stopped."
        fi
        # Say what else changed. This flips a setting that outlives the
        # command, and printing only "Server stopped" is how a caller learns
        # about it later, from hooks that have quietly gone silent.
        if [ "$keep_autostart" = true ]; then
            echo "   Hook auto-start left armed (--keep-autostart)."
        else
            echo "   Hook auto-start is now OFF, so speech will use macOS \`say\`"
            echo "   until you run: ava engine start"
        fi
        ;;
    status)
        if server_pidfile_alive "$PID_FILE"; then
            echo "🟢 mlx-engine server is RUNNING (PID: $(cat "$PID_FILE"))"
        else
            echo "🔴 mlx-engine server is STOPPED"
        fi
        ;;
    *)
        echo "Usage: $0 {start|stop|status}"
        exit 1
        ;;
esac
