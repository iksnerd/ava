#!/bin/bash
# Shared setup for ava's voice scripts (server control, speak, hooks).
# Source this — never execute it directly.

# Claude Code hooks run with a minimal PATH (no shell rc sourced), so make
# sure the dirs our dependencies live in (uv is under ~/.local/bin, not a
# default system dir) are on it. Appended, not prepended: the caller's PATH
# still wins, which is what lets a test put a stub binary in front.
export PATH="$PATH:/opt/homebrew/bin:/usr/local/bin:$HOME/.local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
SPEAK="$SCRIPT_DIR/speak.sh"

# Settings tunable live from the Ava menu bar app (speed, volume,
# voice, snippet lengths). An env var of the same shape always wins, so
# manual testing (`TTS_SPEED=1.0 ./speak.sh ...`) can still override it.
#
# voice-defaults.json is the single source of truth for default values —
# VoiceSettings.swift reads the same file, so a default only ever needs to
# change in one place instead of at every call site in both languages.
# Both paths are overridable so the contract test (internal/voiceconfig) can
# point this reader and the Go one at the same fixture and compare answers.
# Nothing in normal operation sets them; see VOICECONFIG_PATH on the Go side.
VOICE_CONFIG_FILE="${VOICE_CONFIG_FILE:-$HOME/Library/Application Support/ava/config.json}"
VOICE_DEFAULTS_FILE="${VOICE_DEFAULTS_FILE:-$SCRIPT_DIR/voice-defaults.json}"

# config_get <jsonKey> -> the live config's value, else voice-defaults.json's, else empty.
config_get() {
    python3 -c "
import json, sys
live_path, defaults_path, key = sys.argv[1], sys.argv[2], sys.argv[3]
for path in (live_path, defaults_path):
    try:
        with open(path) as f:
            v = json.load(f).get(key)
        if v is not None:
            print(v)
            sys.exit(0)
    except Exception:
        pass
print('')
" "$VOICE_CONFIG_FILE" "$VOICE_DEFAULTS_FILE" "$1"
}

# config_get_int <jsonKey> <lastResortDefault> -> like config_get, but
# guarantees an integer — falls back to lastResortDefault if the value is
# missing/hand-edited into something non-numeric (e.g. a string or object),
# or if voice-defaults.json itself is unreadable. Safe to use in a bash
# `-gt`/`-lt` comparison without it erroring out.
config_get_int() {
    local v
    v=$(config_get "$1")
    case "$v" in
        ''|*[!0-9-]*) echo "$2" ;;
        *) echo "$v" ;;
    esac
}

# config_get_bool <jsonKey> <lastResortDefault: true|false> -> "true"/"false".
# Python prints a JSON bool as "True"/"False" (capitalized), so this normalizes
# that instead of making every call site remember the gotcha.
config_get_bool() {
    local v
    v=$(config_get "$1")
    case "$v" in
        [Tt]rue|1) echo "true" ;;
        [Ff]alse|0) echo "false" ;;
        *) echo "$2" ;;
    esac
}

# voice_is_muted -> true if the menu bar app's global mute switch is on.
# Single source of truth for "is voice output allowed" so hooks can skip
# their expensive work early (transcript parsing, Ollama summarization) and
# speak.sh can still gate every path that shells out to it directly
# (Read Aloud, Test/Preview) even if a caller forgets to check first.
voice_is_muted() {
    [ "$(config_get_bool muted false)" = "true" ]
}

# config_set_bool <jsonKey> <true|false> -> persists a bool into the live
# config file, merging with whatever's already there (creating the file/its
# parent dir on first write) so unrelated keys are untouched. The shell-side
# counterpart to VoiceSettings.swift's save() — lets mlx-engine-server.sh
# flip engineAutoStart without any help from the Swift app, and
# VoiceSettings picks the change up on its own via its external-change poll.
config_set_bool() {
    local key="$1" value="$2"
    python3 -c "
import json, os, sys
path, key, value = sys.argv[1], sys.argv[2], sys.argv[3] == 'true'
os.makedirs(os.path.dirname(path), exist_ok=True)
try:
    with open(path) as f:
        cfg = json.load(f)
except Exception:
    cfg = {}
cfg[key] = value
with open(path, 'w') as f:
    json.dump(cfg, f, indent=2)
" "$VOICE_CONFIG_FILE" "$key" "$value"
}

# voice_engine_autostart_enabled -> false once the user has explicitly
# stopped the mlx-engine server (menu bar Stop, or `ava engine
# stop`), until they explicitly start it again. Lets speak.sh's on-demand
# auto-start (see below) tell "never started" apart from "user turned it
# off" instead of always reviving the server the instant a hook fires.
voice_engine_autostart_enabled() {
    [ "$(config_get_bool engineAutoStart true)" = "true" ]
}

VOICE_HOOKS_PROJECT="$SCRIPT_DIR/voice_hooks"

# voice_hooks_run <script.py> [args...] -> runs a scripts/voice_hooks/ CLI
# entrypoint (markdown stripping, sentence-aware truncation, Ollama
# summarization — see scripts/voice_hooks/) via `uv run`, with stdin passed
# through untouched. Returns the CLI's exit status; callers MUST handle a
# non-zero return themselves (unsynced venv, uv missing from PATH, etc.) by
# falling back to speaking the raw text rather than silently saying nothing.
voice_hooks_run() {
    local script="$1"; shift
    uv run --project "$VOICE_HOOKS_PROJECT" "$VOICE_HOOKS_PROJECT/$script" "$@"
}

# Reads a Claude Code hook JSON payload from stdin once, exposing:
#   HOOK_INPUT - raw JSON
#   HOOK_CWD   - .cwd field
#   HOOK_REPO  - repo/folder name resolved from HOOK_CWD (may be empty)
read_hook_input() {
    HOOK_INPUT=$(cat)
    HOOK_CWD=$(echo "$HOOK_INPUT" | jq -r '.cwd // empty' 2>/dev/null)
    HOOK_REPO=$("$SCRIPT_DIR/repo-name.sh" "$HOOK_CWD")
}

# Prefixes $1 with "<repo>: " when HOOK_REPO is set (from read_hook_input), then speaks it.
speak_hook_message() {
    local msg="$1"
    if [ -n "$HOOK_REPO" ]; then
        msg="$HOOK_REPO: $msg"
    fi
    "$SPEAK" "$msg"
}

# server_pidfile_alive <pidfile> -> 0 if <pidfile> exists and names a live
# process, 1 otherwise. Shared by any script that tracks a background
# server via a PID file (currently mlx-engine-server.sh's start/stop/status,
# which each need this exact check — previously repeated inline 3 times).
server_pidfile_alive() {
    local pidfile="$1"
    [ -f "$pidfile" ] && kill -0 "$(cat "$pidfile")" 2>/dev/null
}

# server_lock_acquire <lockdir> <stale_sec> -> atomically acquires a mutual-
# exclusion lock via mkdir (atomic on POSIX filesystems), waiting up to ~10s
# for a concurrent holder to finish. The holder's PID is written to
# <lockdir>/pid right after the mkdir. A lock whose holder is dead is
# abandoned (e.g. it crashed) and reclaimed. A live holder is never evicted:
# a start can run `uv sync` and wait 2 minutes for /health, far longer than
# any age cutoff. Only a lock with no pid file (the holder is between mkdir
# and the write, or died there) falls back to the age rule: older than
# <stale_sec> by the lockdir's mtime is abandoned.
# Pair with server_lock_release <lockdir> — ideally via `trap ... EXIT` so a
# crash mid-critical-section doesn't wedge it forever.
server_lock_acquire() {
    local lockdir="$1" stale_sec="$2"
    local waited=0
    while ! mkdir "$lockdir" 2>/dev/null; do
        if [ -d "$lockdir" ]; then
            local owner stale=0
            owner=$(cat "$lockdir/pid" 2>/dev/null)
            if [ -n "$owner" ]; then
                kill -0 "$owner" 2>/dev/null || stale=1
            else
                local mtime age
                mtime=$(stat -f %m "$lockdir" 2>/dev/null || echo 0)
                age=$(( $(date +%s) - mtime ))
                [ "$age" -gt "$stale_sec" ] && stale=1
            fi
            if [ "$stale" = 1 ]; then
                # Reclaim by renaming the lockdir aside: rename of a directory
                # is atomic, so of several waiters that judged the same dead
                # owner stale exactly one mv succeeds. The losers' mv fails
                # (source gone) and they fall through to wait and retry mkdir.
                # A plain rmdir here would let a slow waiter remove a live new
                # owner's lock that a faster waiter had already re-created.
                local aside="$lockdir.stale.$$"
                if mv "$lockdir" "$aside" 2>/dev/null; then
                    if [ "$(cat "$aside/pid" 2>/dev/null)" = "$owner" ]; then
                        rm -rf "$aside"
                        continue
                    fi
                    # We moved a lock that is not the one we judged: someone
                    # reclaimed and re-took it between our read and our mv.
                    # Put it back, but only if the path is still free (mv onto
                    # an existing dir would nest into it). Remaining window,
                    # accepted: between our mv and this restore the lock path
                    # is absent, so a third process can mkdir it and run
                    # alongside the displaced owner. That needs a waiter to
                    # stall between two adjacent statements while a reclaim
                    # and a fresh acquire both complete. The same residue
                    # exists for a pidless lock judged stale by age whose
                    # owner writes its pid after our read. Closing it fully
                    # would need a real lock (flock), which macOS lacks.
                    if [ ! -e "$lockdir" ]; then
                        mv "$aside" "$lockdir" 2>/dev/null || rm -rf "$aside"
                    else
                        rm -rf "$aside"
                    fi
                    continue
                fi
            fi
        fi
        waited=$((waited + 1))
        [ "$waited" -gt 50 ] && return 1 # ~10s
        sleep 0.2
    done
    echo "$$" > "$lockdir/pid"
    return 0
}

# server_lock_release <lockdir> -> releases the lock only if this process
# holds it, so a holder whose lock was reclaimed never removes the new
# owner's.
server_lock_release() {
    [ "$(cat "$1/pid" 2>/dev/null)" = "$$" ] || return 0
    rm -f "$1/pid"
    rmdir "$1" 2>/dev/null
}
