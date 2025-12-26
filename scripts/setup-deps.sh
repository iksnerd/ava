#!/bin/bash

# Setup script for installing system dependencies
# Installs sox and whisper-cli via brew

echo "🔧 local-whisper Dependency Setup"
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

echo ""
echo "✅ All dependencies installed!"
exit 0
