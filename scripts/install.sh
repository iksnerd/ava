#!/bin/bash
# Installs the ava CLI binaries from a GitHub release.
#
# Run it rather than downloading in a browser, and Gatekeeper never enters the
# picture: macOS attaches com.apple.quarantine only when an app that opts into
# it does the download, which browsers do and curl and gh do not. Verified on
# macOS 27 — and verified the other way too: quarantine *does* survive
# `tar xzf` there, so extracting a browser-downloaded archive in a terminal
# does not launder it, whatever older advice says.
#
# That matters more than it sounds. These binaries are unsigned (signing for
# distribution needs an Apple Developer Program membership this project does
# not have), and a quarantined unsigned binary run from a shell is killed
# outright: exit 137, no message on stdout or stderr, nothing to search for.
#
# Usage:
#   bash scripts/install.sh              # latest release
#   bash scripts/install.sh v0.3.0       # a specific tag
#   AVA_BIN=~/bin bash scripts/install.sh
set -euo pipefail

REPO="iksnerd/ava"
# LOCAL_WHISPER_BIN is the name before the 0.6.0 rename, still honoured.
BIN_DIR="${AVA_BIN:-${LOCAL_WHISPER_BIN:-$HOME/.local/bin}}"
TAG="${1:-}"

if [ "$(uname -s)" != "Darwin" ] || [ "$(uname -m)" != "arm64" ]; then
    echo "❌ ava is macOS on Apple Silicon only."
    echo "   It shells out to afplay, pbcopy, osascript and sox, and the TTS"
    echo "   engine is MLX, which has no Intel build."
    exit 1
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# gh first: it carries the caller's credentials, so this works while the repo
# is private. curl covers a machine without gh once it is public.
if command -v gh >/dev/null 2>&1; then
    echo "⬇️  Fetching the release with gh..."
    # A release tagged before the GoReleaser job existed carries notes and no
    # binaries, and gh's own "no assets to download" does not say which release
    # it looked at or what to do about it.
    if ! gh release download ${TAG:+"$TAG"} --repo "$REPO" --dir "$tmp" \
        --pattern '*.tar.gz' --pattern 'checksums.txt' 2>"$tmp/gh.err"; then
        if grep -q "no assets" "$tmp/gh.err"; then
            echo "❌ Release ${TAG:-(latest)} has no binaries attached."
            echo "   Releases tagged before the build job existed carry notes only."
            echo "   Pick a newer tag, or see CONTRIBUTING.md on cutting a release."
        else
            sed 's/^/   /' "$tmp/gh.err"
        fi
        exit 1
    fi
elif command -v curl >/dev/null 2>&1; then
    [ -n "$TAG" ] || TAG="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
        sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)"
    if [ -z "$TAG" ]; then
        echo "❌ Could not work out the latest tag. If the repo is still private,"
        echo "   install the GitHub CLI (brew install gh) and re-run — it carries"
        echo "   your credentials where a plain curl cannot."
        exit 1
    fi
    version="${TAG#v}"
    echo "⬇️  Downloading $TAG..."
    base="https://github.com/$REPO/releases/download/$TAG"
    # Releases before 0.6.0 were published as local-whisper_<version>_...
    curl -fsSL -o "$tmp/ava.tar.gz" "$base/ava_${version}_darwin_arm64.tar.gz" 2>/dev/null ||
        curl -fsSL -o "$tmp/ava.tar.gz" "$base/local-whisper_${version}_darwin_arm64.tar.gz"
    curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt"
else
    echo "❌ Neither gh nor curl is available to download with."
    exit 1
fi

archive="$(find "$tmp" -name '*.tar.gz' -maxdepth 1 | head -1)"
if [ -z "$archive" ]; then
    echo "❌ The release carried no .tar.gz. Has a release been published yet?"
    exit 1
fi

# Verify before extracting, not after: an archive that fails its checksum
# should never have had its contents on disk.
if [ -f "$tmp/checksums.txt" ]; then
    echo "🔍 Verifying checksum..."
    (cd "$tmp" && shasum -a 256 -c --ignore-missing checksums.txt >/dev/null) || {
        echo "❌ Checksum mismatch — refusing to install."
        exit 1
    }
    echo "✅ Checksum verified."
else
    echo "⚠️  No checksums.txt in the release; installing unverified."
fi

tar xzf "$archive" -C "$tmp"
mkdir -p "$BIN_DIR"
# The old names cover installing a release from before the 0.6.0 rename.
for binary in ava ava-monitor local-whisper voice-monitor; do
    [ -f "$tmp/$binary" ] || continue
    install -m 0755 "$tmp/$binary" "$BIN_DIR/$binary"
    # Belt and braces: nothing above should have set quarantine, and if
    # something did, the failure it causes is a silent kill with no message.
    xattr -d com.apple.quarantine "$BIN_DIR/$binary" 2>/dev/null || true
    echo "✅ Installed $binary to $BIN_DIR"
done

# The CLI was called local-whisper until 0.6.0. The alias keeps an MCP
# registration, a Raycast launcher or an Ava.app built against the old name
# working for one release; it goes away in 0.7.0.
if [ -f "$BIN_DIR/ava" ]; then
    ln -sf ava "$BIN_DIR/local-whisper"
    echo "✅ Linked local-whisper -> ava (the old name, kept for one release)"
fi

echo ""
case ":$PATH:" in
    *":$BIN_DIR:"*) ;;
    *)  echo "⚠️  $BIN_DIR is not on your PATH. Add it:"
        echo "      echo 'export PATH=\"$BIN_DIR:\$PATH\"' >> ~/.zshrc" ;;
esac
echo "Next: ava setup"
echo "   Installs sox, whisper-cli, the speech model and the Kokoro TTS engine."
