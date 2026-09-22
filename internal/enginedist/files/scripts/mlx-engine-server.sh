#!/bin/bash
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"
DIR="$ROOT_DIR"
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

        echo "🚀 Starting local voice MLX server in the background..."
        cd "$DIR/mlx-engine"
        uv sync -q
        # Exec the venv's own uvicorn directly instead of `uv run uvicorn` —
        # `uv run` can fork a child rather than exec into it, in which case
        # $! captures the wrapper's PID, not the real server, and killing it
        # later leaves an orphaned uvicorn process still bound to the port.
        # MLX_ENGINE_PID_FILE tells the server which file to clean up when it
        # shuts itself down on idle. It used to hardcode a different path than
        # this script writes, so a self-exit left the real pid file behind.
        MLX_ENGINE_PID_FILE="$PID_FILE" \
            # Host and port split out of the generated ENGINE_URL, so this cannot
            # bind somewhere the clients are not looking.
            engine_hostport="${ENGINE_URL#*//}"
            nohup .venv/bin/uvicorn server:app \
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
