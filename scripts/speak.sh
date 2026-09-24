#!/bin/bash
# Speak text through `ava speak`: Kokoro on the local mlx-engine, falling back
# to macOS `say`, under the global mute and the shared playback lock.
#
# Usage: speak.sh "text to speak" [voice]
#
# The entry point for the Claude Code hooks and the menu bar app. It used to
# carry its own synthesis, fallback and player, a second copy of what
# internal/speaker does; a stop fix made to one missed the other. Everything
# past the mute check is now the Go speaker's.
#
# Returns at once (`--async`), so it never stalls the hook that invoked it.
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

TEXT="$1"
VOICE="${2:-}"

if [ -z "$TEXT" ]; then
    echo "Usage: $0 \"text to speak\" [voice]" >&2
    exit 1
fi

# `ava speak` warns on stderr while muted; a hook that fires on every turn
# stays quiet instead.
if voice_is_muted; then
    exit 0
fi

# In the order the menu bar app looks for it: the binary an app bundle ships
# beside its copy of scripts/, then the checkout's `make build`, then PATH. A
# ~/.local/bin install goes last because it can go stale, with nothing to
# prompt a rebuild. The bundle's slot applies only inside a bundle: a
# checkout's root holds whatever a bare `go build` left there.
bundled=""
case "$ROOT_DIR" in */Contents/Resources) bundled="$ROOT_DIR/ava" ;; esac
AVA=""
for candidate in "$bundled" "$ROOT_DIR/bin/ava" "$(command -v ava)"; do
    if [ -n "$candidate" ] && [ -x "$candidate" ] && [ ! -d "$candidate" ]; then
        AVA="$candidate"
        break
    fi
done
if [ -z "$AVA" ]; then
    echo "speak.sh: no ava binary to speak with. Run \`make build\` in this checkout," >&2
    echo "or install a release (see docs/getting-started.md)." >&2
    exit 1
fi

args=(speak --async)
[ -n "$VOICE" ] && args+=(--voice "$VOICE")
exec "$AVA" "${args[@]}" -- "$TEXT"
