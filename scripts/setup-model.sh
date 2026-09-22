#!/bin/bash

# Setup script for downloading Whisper model
# Downloads base model if it doesn't exist

MODEL_DIR="$HOME/.local/share/whisper-cpp"
MODEL_FILE="$MODEL_DIR/ggml-base.en.bin"
# Pinned to a commit, with its sha256, to match `local-whisper setup-model`
# (cmd/local-whisper/setup_model.go). resolve/main is a mutable ref.
MODEL_REVISION="5359861c739e955e79d9a303bcbc70fb988958b1"
MODEL_SHA256="a03779c86df3323075f5e796cb2ce5029f00ec8869eee3fdfb897afe36c6d002"
MODEL_URL="https://huggingface.co/ggerganov/whisper.cpp/resolve/$MODEL_REVISION/ggml-base.en.bin"
MODEL_SIZE="141MB"

echo "🎙️  local-whisper Model Setup"
echo ""

# Create directory
mkdir -p "$MODEL_DIR"
if [ $? -ne 0 ]; then
    echo "❌ Failed to create model directory: $MODEL_DIR"
    exit 1
fi

# Check if model already exists
if [ -f "$MODEL_FILE" ]; then
    FILE_SIZE=$(du -h "$MODEL_FILE" | cut -f1)
    echo "✅ Model already exists: $MODEL_FILE ($FILE_SIZE)"
    exit 0
fi

# Download model
echo "⬇️  Downloading Whisper base model (~$MODEL_SIZE)..."
echo "   From: $MODEL_URL"
echo "   To: $MODEL_FILE"
echo ""

if command -v wget &> /dev/null; then
    wget -O "$MODEL_FILE" "$MODEL_URL"
    DOWNLOAD_STATUS=$?
elif command -v curl &> /dev/null; then
    curl -L -o "$MODEL_FILE" "$MODEL_URL"
    DOWNLOAD_STATUS=$?
else
    echo "❌ Neither wget nor curl found. Please install one:"
    echo "   brew install wget"
    exit 1
fi

if [ $DOWNLOAD_STATUS -eq 0 ] && ! echo "$MODEL_SHA256  $MODEL_FILE" | shasum -a 256 -c --status; then
    echo "❌ Checksum mismatch: $MODEL_FILE is not the expected model"
    DOWNLOAD_STATUS=1
fi

if [ $DOWNLOAD_STATUS -eq 0 ]; then
    FILE_SIZE=$(du -h "$MODEL_FILE" | cut -f1)
    echo ""
    echo "✅ Model downloaded successfully: $FILE_SIZE"
    exit 0
else
    echo "❌ Failed to download model"
    rm -f "$MODEL_FILE"
    exit 1
fi
