#!/bin/bash
# Fails if any tracked file hardcodes somebody's home directory.
#
# This repo is public and gets cloned to paths that are not this machine's. A
# hardcoded "/Users/<name>/..." works forever for the author and breaks on the
# first checkout by anyone else, silently: a PATH entry that does not exist is
# not an error, it is just a lookup that fails later somewhere unrelated.
#
# Both hits that prompted this check were exactly that shape - a PATH entry in
# scripts/lib.sh and a dev-checkout fallback in Paths.swift.
#
# Run by `make lint`. Use $HOME, a path relative to the script, or Swift's
# #filePath instead.
set -uo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.." || exit 1

# Split so this file does not match its own pattern.
pattern="/Users/[a-z]\|/home/[a-z]"

hits=$(git ls-files -z | xargs -0 grep -n "$pattern" 2>/dev/null | grep -v "^scripts/check-portable-paths.sh:")

if [ -n "$hits" ]; then
    echo "❌ Hardcoded home directories in tracked files:"
    echo "$hits" | sed 's/^/   /'
    echo ""
    echo "   Use \$HOME, a path relative to the script, or #filePath (Swift)."
    exit 1
fi

echo "✅ No hardcoded home directories"
