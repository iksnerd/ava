#!/bin/bash

# Setup script for Voxtral MLX primitives (Mini 3B STT, Mini 4B Realtime STT, 4B TTS)
# Uses uv to create voxtral/.venv from voxtral/pyproject.toml. uv caches
# downloaded packages globally, so re-running this after editing
# pyproject.toml only re-fetches what actually changed.
# Model weights are NOT downloaded here - they pull from Hugging Face on
# first use of each primitive (a few GB each).

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VOXTRAL_DIR="$REPO_ROOT/voxtral"

echo "🗣️  Voxtral MLX Setup"
echo ""

if [[ "$(uname -m)" != "arm64" ]]; then
    echo "❌ MLX requires Apple Silicon (arm64). Detected: $(uname -m)"
    exit 1
fi

if ! command -v uv &> /dev/null; then
    echo "⬇️  Installing uv (Python package/venv manager)..."
    if command -v brew &> /dev/null; then
        brew install uv
    else
        curl -LsSf https://astral.sh/uv/install.sh | sh
    fi
    if ! command -v uv &> /dev/null; then
        echo "❌ Failed to install uv. Install it manually: https://docs.astral.sh/uv/"
        exit 1
    fi
fi

echo "⬇️  Syncing Voxtral environment (uv sync)..."
( cd "$VOXTRAL_DIR" && uv sync )
if [ $? -ne 0 ]; then
    echo "❌ Failed to sync Voxtral dependencies"
    exit 1
fi

echo ""
echo "✅ Voxtral environment ready: $VOXTRAL_DIR/.venv"
echo ""
echo "Try it:"
echo "  cd voxtral && uv run tts.py --text \"Hello from Voxtral\" --output /tmp/hello.wav && afplay /tmp/hello.wav"
echo ""
echo "Notes:"
echo "  - Model weights download automatically on first use (a few GB each)."
echo "  - Voxtral TTS weights are licensed CC-BY-NC (non-commercial use only)."
exit 0
