#!/bin/bash
# Shared setup for local-whisper's voice scripts (server control, speak, hooks).
# Source this — never execute it directly.

# Claude Code hooks run with a minimal PATH (no shell rc sourced), so pin the
# dirs our dependencies actually live in (uv is under ~/.local/bin, not a
# default system dir) regardless of who invokes us.
export PATH="/opt/homebrew/bin:/usr/local/bin:/Users/user/.local/bin:/usr/bin:/bin:/usr/sbin:/sbin:$PATH"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
SPEAK="$SCRIPT_DIR/speak.sh"

# Settings tunable live from the Claude Voice menu bar app (speed, volume,
# voice, snippet lengths). An env var of the same shape always wins, so
# manual testing (`TTS_SPEED=1.0 ./speak.sh ...`) can still override it.
#
# voice-defaults.json is the single source of truth for default values —
# VoiceSettings.swift reads the same file, so a default only ever needs to
# change in one place instead of at every call site in both languages.
VOICE_CONFIG_FILE="$HOME/Library/Application Support/ClaudeVoice/config.json"
VOICE_DEFAULTS_FILE="$SCRIPT_DIR/voice-defaults.json"

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
