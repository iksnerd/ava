#!/bin/bash
# Fails if any tracked file has a name that breaks somebody else's checkout.
#
# Every other guard in this repo reads file *contents*. Nothing read a *name*,
# which is how a file called
#
#     c -l)|count=$(list_files \| tr '\0' '\n' \| grep -c .)|
#
# — a fragment of check-portable-paths.sh that a mistyped redirect turned into a
# filename — was committed, pushed, and survived every green `make lint` run
# until somebody happened to look at the root directory listing.
#
# Two things make that worth a check rather than a shrug. `|`, `:`, `*`, `?`,
# `<`, `>` and `"` are illegal in NTFS filenames, so the clone fails outright on
# Windows rather than degrading. And the same accident quietly produces junk in
# any repo where a shell command building a file list goes wrong, which is a
# thing that happens more than once.
#
# Run by `make lint` and by the pre-commit hook.
set -uo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.." || exit 1

# Same git-then-find fallback as check-portable-paths.sh, for the same reason:
# the pre-commit hook runs this against a `git checkout-index` snapshot that has
# no .git, where `git ls-files` fails and would hand back an empty list.
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

fail=0
report() {
    [ "$fail" -eq 0 ] && echo "❌ Filenames that will not survive another checkout:"
    fail=1
    printf '   %s\n      %s\n' "$1" "$2"
}

# Windows reserved device names, which cannot exist as a file of any extension.
reserved="CON PRN AUX NUL COM1 COM2 COM3 COM4 COM5 COM6 COM7 COM8 COM9 LPT1 LPT2 LPT3 LPT4 LPT5 LPT6 LPT7 LPT8 LPT9"

while IFS= read -r -d '' path; do
    # Check each path segment, not the whole path: `/` is legal, the rest is not.
    printf '%s\n' "${path#./}" | tr '/' '\n' | while IFS= read -r seg; do
        [ -n "$seg" ] || continue
        printf '%s\t%s\n' "$path" "$seg"
    done
done < <(list_files) > /tmp/.check-filenames.$$ || true

while IFS=$'\t' read -r path seg; do
    case "$seg" in
        *[!A-Za-z0-9._-]*)
            report "$path" "segment \"$seg\" has a character outside A-Za-z0-9._- ; \`|\` \`:\` \`*\` \`?\` are also illegal on Windows" ;;
        -*)
            report "$path" "segment \"$seg\" starts with a dash, which every CLI reads as a flag" ;;
        *.)
            report "$path" "segment \"$seg\" ends with a dot, which Windows silently strips" ;;
    esac
    base="${seg%%.*}"
    upper=$(printf '%s' "$base" | tr 'a-z' 'A-Z')
    case " $reserved " in
        *" $upper "*) report "$path" "\"$base\" is a reserved device name on Windows" ;;
    esac
done < /tmp/.check-filenames.$$
rm -f /tmp/.check-filenames.$$

# Two files differing only in case cannot both exist on a case-insensitive
# filesystem — which is the macOS default, so this breaks a clone here too.
#
# This branch cannot be exercised by creating the files on a Mac: the second
# redirect overwrites the first instead of making a second file. That is the
# point — the pair arrives from a contributor on a case-sensitive filesystem,
# and the clone here is what breaks. Verify the logic directly instead:
#   printf 'a/README.md\na/readme.md\n' | tr 'A-Z' 'a-z' | sort | uniq -d
collisions=$(list_files | tr '\0' '\n' | tr 'A-Z' 'a-z' | sort | uniq -d)
if [ -n "$collisions" ]; then
    [ "$fail" -eq 0 ] && echo "❌ Filenames that will not survive another checkout:"
    fail=1
    echo "   paths differing only by case, which collide on a case-insensitive filesystem:"
    echo "$collisions" | sed 's/^/      /'
fi

if [ "$fail" -ne 0 ]; then
    echo ""
    echo "   Rename the file. If a command created it by accident, delete it."
    exit 1
fi

echo "✅ Filenames are portable ($count files checked)"
