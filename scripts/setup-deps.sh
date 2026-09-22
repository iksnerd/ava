#!/bin/bash

# Setup script for installing system dependencies
# Installs sox and whisper-cli via brew

echo "🔧 ava Dependency Setup"
echo ""

# Check if brew is installed
if ! command -v brew &> /dev/null; then
    echo "❌ Homebrew not found. Please install it first:"
    echo "   /bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""
    exit 1
fi

# Install sox
if ! command -v sox &> /dev/null; then
    echo "⬇️  Installing sox..."
    brew install sox
    if [ $? -ne 0 ]; then
        echo "❌ Failed to install sox"
        exit 1
    fi
    echo "✅ sox installed"
else
    echo "✅ sox already installed"
fi

# Install whisper-cli
if ! command -v whisper-cli &> /dev/null; then
    echo "⬇️  Installing whisper-cli..."
    brew install whisper-cpp
    if [ $? -ne 0 ]; then
        echo "❌ Failed to install whisper-cli"
        exit 1
    fi
    echo "✅ whisper-cli installed"
else
    echo "✅ whisper-cli already installed"
fi

# Setup Python and MLX dependencies
if ! command -v uv &> /dev/null; then
    echo "⬇️  Installing uv..."
    curl -LsSf https://astral.sh/uv/install.sh | sh
    if [ $? -ne 0 ]; then
        echo "❌ Failed to install uv"
        exit 1
    fi
    export PATH="$HOME/.local/bin:$PATH"
    echo "✅ uv installed"
else
    echo "✅ uv already installed"
fi

# mlx-engine is MLX, so Apple Silicon only. The README promises the default
# engine works on any Mac, and it does — sox and whisper-cli above are the whole
# of it. Attempting this sync on an Intel Mac failed the entire setup at the last
# step, after everything that actually mattered had already succeeded.
if [ "$(uname -m)" != "arm64" ]; then
    echo "⏭️  Skipping mlx-engine (Apple Silicon only — MLX has no Intel build)."
    echo "    Dictation is fully set up and needs nothing else: sox and"
    echo "    whisper-cli above are the whole of it."
    echo "    Kokoro TTS and call monitoring are unavailable on this Mac;"
    echo "    speech falls back to the macOS \`say\` voice."
    exit 0
fi

echo "⬇️  Installing mlx-engine (Kokoro TTS) dependencies..."
if ! (cd mlx-engine && uv sync); then
    echo "❌ Failed to setup mlx-engine dependencies"
    exit 1
fi
echo "✅ mlx-engine dependencies installed"

echo ""
echo "✅ All dependencies installed!"
exit 0
