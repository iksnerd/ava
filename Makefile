.PHONY: build install-raycast install-bin clean help

BINARY_NAME=local-whisper
RAYCAST_DIR=$(HOME)/raycast-scripts
BIN_DIR=$(HOME)/.local/bin

help:
	@echo "local-whisper Makefile"
	@echo ""
	@echo "Commands:"
	@echo "  make build              - Build the binary"
	@echo "  make install-raycast    - Install as Raycast command script"
	@echo "  make install-bin        - Install binary to ~/.local/bin"
	@echo "  make clean              - Remove built binary"
	@echo ""

build:
	@echo "🔨 Building $(BINARY_NAME)..."
	@go build -o $(BINARY_NAME)
	@echo "✅ Built: ./$(BINARY_NAME)"

install-raycast: build
	@echo "📦 Installing Raycast command..."
	@mkdir -p $(RAYCAST_DIR)
	@echo "#!/bin/bash" > $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "# @raycast.schemaVersion 1" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "# @raycast.title Transcribe Local Whisper" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "# @raycast.description Voice transcription using local Whisper model (no cloud, no data sent)" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "# @raycast.mode fullOutput" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "# @raycast.currentDirectoryPath" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "# @raycast.icon 🎙️" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "# @raycast.packageName Voice" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "# @raycast.author iksnerd" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "# Local whisper voice transcription" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "cd \"\$${RAYCAST_CURRENT_DIRECTORY_PATH:-.}\" || exit 1" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "exec $(abspath $(BINARY_NAME))" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@chmod +x $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "✅ Installed: $(RAYCAST_DIR)/whisper-transcribe.sh"
	@echo ""
	@echo "Next steps:"
	@echo "1. Open Raycast Settings (Cmd+,)"
	@echo "2. Go to Extensions"
	@echo "3. Click '(+) Add Script Directory'"
	@echo "4. Select: $(RAYCAST_DIR)"
	@echo "5. Reload Raycast (Cmd+Shift+R)"

install-bin: build
	@echo "📦 Installing binary..."
	@mkdir -p $(BIN_DIR)
	@cp $(BINARY_NAME) $(BIN_DIR)/$(BINARY_NAME)
	@chmod +x $(BIN_DIR)/$(BINARY_NAME)
	@echo "✅ Installed: $(BIN_DIR)/$(BINARY_NAME)"
	@echo ""
	@echo "Add to your PATH in ~/.zshrc or ~/.bash_profile:"
	@echo "  export PATH=\"$(BIN_DIR):\$$PATH\""

clean:
	@echo "🧹 Cleaning up..."
	@rm -f $(BINARY_NAME)
	@echo "✅ Cleaned"
