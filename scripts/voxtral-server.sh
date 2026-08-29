#!/bin/bash
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"
DIR="$ROOT_DIR"
PID_FILE="/tmp/voxtral-server.pid"
LOG_FILE="/tmp/voxtral-server.log"
START_LOCKDIR="/tmp/voxtral-server-start.lockdir"
START_LOCK_STALE_SEC=30

# Multiple Claude Code sessions can all find the server down at once and race
# to start it. mkdir is atomic on POSIX filesystems, so it doubles as a lock:
# only the session that creates the dir proceeds, everyone else waits for it
# to finish (by which point the PID file / health check reflects the real
# server) instead of double-spawning or clobbering the PID file.
acquire_start_lock() {
    local waited=0
    while ! mkdir "$START_LOCKDIR" 2>/dev/null; do
        if [ -d "$START_LOCKDIR" ]; then
            local mtime age
            mtime=$(stat -f %m "$START_LOCKDIR" 2>/dev/null || echo 0)
            age=$(( $(date +%s) - mtime ))
            if [ "$age" -gt "$START_LOCK_STALE_SEC" ]; then
                rmdir "$START_LOCKDIR" 2>/dev/null
                continue
            fi
        fi
        waited=$((waited + 1))
        [ "$waited" -gt 50 ] && return 1 # ~10s
        sleep 0.2
    done
    return 0
}

release_start_lock() {
    rmdir "$START_LOCKDIR" 2>/dev/null
}

case "$1" in
    start)
        if ! acquire_start_lock; then
            echo "❌ Timed out waiting for another 'start' to finish."
            exit 1
        fi
        trap release_start_lock EXIT

        if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
            echo "✅ Voxtral server is already running (PID: $(cat "$PID_FILE"))"
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
        if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
            PID=$(cat "$PID_FILE")
            echo "🛑 Stopping Voxtral server (PID: $PID)..."
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
        if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
            echo "🟢 Voxtral server is RUNNING (PID: $(cat "$PID_FILE"))"
        else
            echo "🔴 Voxtral server is STOPPED"
        fi
        ;;
    *)
        echo "Usage: $0 {start|stop|status}"
        exit 1
        ;;
esac
