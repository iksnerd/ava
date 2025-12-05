.PHONY: build test install-raycast install-bin setup-model clean help

BINARY_NAME=local-whisper
BUILD_DIR=bin
RAYCAST_DIR=$(HOME)/raycast-scripts
INSTALL_BIN_DIR=$(HOME)/.local/bin
BINARY_PATH=$(BUILD_DIR)/$(BINARY_NAME)

help:
	@echo "local-whisper Makefile"
	@echo ""
	@echo "Commands:"
	@echo "  make build              - Build the binary to bin/"
	@echo "  make test               - Run all tests"
	@echo "  make setup-model        - Download Whisper model to ~/.local/share/whisper-cpp/"
	@echo "  make install-raycast    - Install as Raycast command script (includes model setup)"
	@echo "  make install-bin        - Install binary to ~/.local/bin (includes model setup)"
	@echo "  make clean              - Remove bin/ directory"
	@echo ""

build:
	@echo "🔨 Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@cd cmd/local-whisper && go build -o ../../$(BINARY_PATH)
	@echo "✅ Built: ./$(BINARY_PATH)"

test:
	@echo "🧪 Running tests..."
	@go test -v ./...
	@echo "✅ Tests passed"

setup-model:
	@bash scripts/setup-model.sh

install-raycast: build setup-model
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
	@echo "exec $(abspath $(BINARY_PATH))" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@chmod +x $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "✅ Installed: $(RAYCAST_DIR)/whisper-transcribe.sh"
	@echo ""
	@echo "Next steps:"
	@echo "1. Open Raycast Settings (Cmd+,)"
	@echo "2. Go to Extensions"
	@echo "3. Click '(+) Add Script Directory'"
	@echo "4. Select: $(RAYCAST_DIR)"
	@echo "5. Reload Raycast (Cmd+Shift+R)"

install-bin: build setup-model
	@echo "📦 Installing binary..."
	@mkdir -p $(INSTALL_BIN_DIR)
	@cp $(BINARY_PATH) $(INSTALL_BIN_DIR)/$(BINARY_NAME)
	@chmod +x $(INSTALL_BIN_DIR)/$(BINARY_NAME)
	@echo "✅ Installed: $(INSTALL_BIN_DIR)/$(BINARY_NAME)"
	@echo ""
	@echo "Setup complete! You can now use:"
	@echo "  $(BINARY_NAME)"
	@echo ""
	@echo "Optional: Add to your PATH in ~/.zshrc or ~/.bash_profile:"
	@echo "  export PATH=\"$(INSTALL_BIN_DIR):\$$PATH\""

clean:
	@echo "🧹 Cleaning up..."
	@rm -rf $(BUILD_DIR)
	@echo "✅ Cleaned"
