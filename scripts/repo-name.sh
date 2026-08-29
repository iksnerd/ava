#!/bin/bash
# Prints a short human name for the repo/project at the given path (arg 1):
# the git repo's directory name if it's inside a git repo, else the
# directory's own basename. Prints nothing if the path is missing/invalid.
CWD="$1"

if [ -z "$CWD" ] || [ ! -d "$CWD" ]; then
    exit 0
fi

GIT_ROOT=$(git -C "$CWD" rev-parse --show-toplevel 2>/dev/null)
if [ -n "$GIT_ROOT" ]; then
    basename "$GIT_ROOT"
else
    basename "$CWD"
fi
