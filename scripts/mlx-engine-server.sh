#!/bin/bash
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"
DIR="$ROOT_DIR"
PID_FILE="/tmp/mlx-engine-server.pid"
LOG_FILE="/tmp/mlx-engine-server.log"
START_LOCKDIR="/tmp/mlx-engine-server-start.lockdir"
START_LOCK_STALE_SEC=30

case "$1" in
    start)
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
        if curl -s -f http://127.0.0.1:8765/health > /dev/null 2>&1; then
            echo "✅ Server already responding on 8765 (started outside this script; not touching its PID)."
            exit 0
        fi

        echo "🚀 Starting local voice MLX server in the background..."
        cd "$DIR/mlx-engine"
        uv sync -q
        # Exec the venv's own uvicorn directly instead of `uv run uvicorn` —
        # `uv run` can fork a child rather than exec into it, in which case
        # $! captures the wrapper's PID, not the real server, and killing it
        # later leaves an orphaned uvicorn process still bound to the port.
        nohup .venv/bin/uvicorn server:app --host 127.0.0.1 --port 8765 > "$LOG_FILE" 2>&1 &
        PID=$!
        echo $PID > "$PID_FILE"
        echo "✅ Server started with PID $PID. Logs at $LOG_FILE"

        echo "⏳ Waiting for server process to come up (models load lazily per-endpoint)..."
        for i in {1..120}; do
            if curl -s -f http://127.0.0.1:8765/health > /dev/null; then
                echo "🎉 Server is up and ready!"
                exit 0
            fi
            sleep 1
        done
        echo "❌ Server failed to start in time. Check $LOG_FILE"
        exit 1
        ;;
    stop)
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
