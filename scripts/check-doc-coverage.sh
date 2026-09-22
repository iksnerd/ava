#!/bin/bash
# Fails when a shipped Makefile target or scripts/*.sh is mentioned in no
# markdown file.
#
# Written because this repo kept shipping working things nobody could find.
# `make setup-blackhole` existed and was referenced nowhere, so the setup doc
# walked readers through installing BlackHole by hand. `make setup` — the
# one-shot path — was advertised only by `make help`. Both were found by a human
# listing targets and grepping for them, which is a loop a script should run.
#
# The allowlist is for targets a user should never need to type: internal
# plumbing and the aliases other targets compose from.
set -uo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.." || exit 1

# Internal: invoked by other targets or by CI, never by a reader.
ALLOW=" vet fmt fmt-check help clean "

# list_files prints the files to check, NUL-separated.
#
# `git ls-files` is right in a checkout and wrong in the pre-commit hook, which
# runs these checks against a snapshot exported by `git checkout-index` — a
# plain directory with no .git. There, git failed, the file list came back
# empty, and the check printed its success line having examined nothing. A
# planted violation passed. Falling back to `find`, and refusing to report
# success on an empty list, is what makes the hook's green tick mean something.
list_files() {
    if git rev-parse --git-dir >/dev/null 2>&1; then
        git ls-files -z
    else
        find . -type f -print0 \
            | grep -zv '/\.git/\|/\.venv/\|/\.build/\|/__pycache__/\|/node_modules/\|/\.ruff_cache/\|/\.pytest_cache/'
    fi
}

require_nonempty() {
    if [ "$1" -eq 0 ]; then
        echo "❌ $2: found no files to check — refusing to report success."
        echo "   This is the vacuous pass this guard exists to prevent."
        exit 1
    fi
}

# Markdown corpus, read once. grep -F against this instead of `git grep`, which
# does not work in the hook's exported snapshot.
DOCS="$(list_files | xargs -0 grep -lI '' 2>/dev/null | grep -E '\.md$' || true)"
require_nonempty "$(printf '%s\n' "$DOCS" | grep -c . || true)" "check-doc-coverage"
mentions() { printf '%s\n' "$DOCS" | xargs grep -qF -- "$1" 2>/dev/null; }

missing=""

while read -r target; do
    case "$ALLOW" in *" $target "*) continue;; esac
    mentions "make $target" || missing="$missing  make $target"$'\n'
done < <(grep -oE '^[a-z][a-z0-9-]*:' Makefile | tr -d ':' | sort -u)

for script in scripts/*.sh; do
    base="$(basename "$script")"
    stem="${base%.sh}"
    # Documented either by filename or via the `make` target that wraps it.
    mentions "$base" && continue
    mentions "make $stem" && continue
    missing="$missing  $script"$'\n'
done

if [ -n "$missing" ]; then
    echo "❌ Shipped but mentioned in no .md file:"
    printf '%s' "$missing"
    echo
    echo "   Document it, or add it to ALLOW in $0 if a reader never needs it."
    exit 1
fi

echo "✅ Every make target and script is documented"
