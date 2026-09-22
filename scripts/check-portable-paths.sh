#!/bin/bash
# Fails if any tracked file hardcodes somebody's home directory.
#
# This repo gets cloned to paths that are not this machine's. A hardcoded
# "/Users/<name>/..." works forever for the author and breaks on the first
# checkout by anyone else, silently: a PATH entry that does not exist is not an
# error, it is a lookup that fails later somewhere unrelated.
#
# Both hits that prompted this check were exactly that shape — a PATH entry in
# scripts/lib.sh and a dev-checkout fallback in Paths.swift.
#
# Run by `make lint` and by the pre-commit hook. Use $HOME, a path relative to
# the script, or Swift's #filePath instead.
set -uo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.." || exit 1

# list_files prints the files to check, NUL-separated.
#
# `git ls-files` is correct in a checkout and wrong in the pre-commit hook, which
# runs this against a snapshot exported by `git checkout-index` — a plain
# directory with no .git. There, git failed, the file list came back empty, and
# this script printed its success line having examined nothing. A planted
# violation passed. Every green tick the hook showed was meaningless.
list_files() {
    if git rev-parse --git-dir >/dev/null 2>&1; then
        git ls-files -z
    else
        find . -type f -print0 | grep -zv \
            -e '/\.git/' -e '/\.venv/' -e '/\.build/' -e '/__pycache__/' \
            -e '/node_modules/' -e '/\.ruff_cache/' -e '/\.pytest_cache/' \
            -e '/\.idea/' -e '/\.vscode/' -e '/\.DS_Store'
    fi
}

count=$(list_files | tr '\0' '\n' | grep -c .)
if [ "$count" -eq 0 ]; then
    echo "❌ Found no files to check — refusing to report success."
    echo "   That vacuous pass is the failure this guard exists to prevent."
    exit 1
fi

# Split so this file does not match its own pattern. Case-insensitive on the
# username: the original pattern was /Users/[a-z] and missed /Users/Admin.
pattern="/Users/[a-zA-Z]\|/home/[a-zA-Z]"

hits=$(list_files | xargs -0 grep -nI "$pattern" 2>/dev/null \
    | grep -v "check-portable-paths.sh:")

if [ -n "$hits" ]; then
    echo "❌ Hardcoded home directories:"
    echo "$hits" | sed 's/^/   /'
    echo ""
    echo "   Use \$HOME, a path relative to the script, or #filePath (Swift)."
    exit 1
fi

echo "✅ No hardcoded home directories ($count files checked)"
