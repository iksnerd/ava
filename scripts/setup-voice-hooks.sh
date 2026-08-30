#!/bin/bash
# Setup for the voice_hooks Python package (markdown stripping, sentence-aware
# truncation, Ollama summarization used by hook-stop.sh / hook-notify.sh).
# Assumes uv is already installed — run `make setup-deps` first if not.

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VOICE_HOOKS_DIR="$REPO_ROOT/scripts/voice_hooks"

echo "🔊 Voice hooks Python setup"
echo ""

if ! command -v uv &> /dev/null; then
    echo "❌ uv not found. Run 'make setup-deps' first."
    exit 1
fi

echo "⬇️  Syncing voice_hooks environment (uv sync)..."
( cd "$VOICE_HOOKS_DIR" && uv sync )
if [ $? -ne 0 ]; then
    echo "❌ Failed to sync voice_hooks dependencies"
    exit 1
fi

echo ""
echo "✅ voice_hooks environment ready: $VOICE_HOOKS_DIR/.venv"
echo "Try it: cd scripts/voice_hooks && uv run pytest"
exit 0
