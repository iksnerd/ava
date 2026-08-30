.PHONY: build build-voice-monitor test vet fmt fmt-check lint install-raycast install-bin setup-deps setup-model setup setup-voxtral setup-voice-hooks setup-blackhole clean help

BINARY_NAME=local-whisper
MONITOR_BINARY_NAME=voice-monitor
BUILD_DIR=bin
RAYCAST_DIR=$(HOME)/raycast-scripts
INSTALL_BIN_DIR=$(HOME)/.local/bin
BINARY_PATH=$(BUILD_DIR)/$(BINARY_NAME)
MONITOR_BINARY_PATH=$(BUILD_DIR)/$(MONITOR_BINARY_NAME)

help:
	@echo "local-whisper Makefile"
	@echo ""
	@echo "Commands:"
	@echo "  make setup              - Install all dependencies (sox, whisper-cli) and download model"
	@echo "  make setup-deps         - Install system dependencies (sox, whisper-cli, uv)"
	@echo "  make setup-model        - Download Whisper model to ~/.local/share/whisper-cpp/"
	@echo "  make setup-voxtral      - Create Python venv and install Voxtral MLX primitives"
	@echo "  make setup-voice-hooks  - Create Python venv for the voice-hook text/summarization CLIs"
	@echo "  make setup-blackhole    - Install BlackHole loopback driver to capture system/call audio"
	@echo "  make build              - Build the binary to bin/"
	@echo "  make build-voice-monitor - Build the realtime transcript monitor (localhost + log file)"
	@echo "  make test               - Run all tests"
	@echo "  make vet                - go vet the Go code"
	@echo "  make fmt                - Format Go (gofmt) and Python (ruff format), in place"
	@echo "  make fmt-check          - Check formatting without modifying files (CI-safe)"
	@echo "  make lint               - vet + fmt-check + ruff check (mlx-engine/, voxtral/, scripts/voice_hooks/)"
	@echo "  make start-engine       - Start the Voxtral MLX background server"
	@echo "  make stop-engine        - Stop the Voxtral MLX background server"
	@echo "  make status-engine      - Check if the Voxtral MLX server is running"
	@echo "  make install-raycast    - Install as Raycast command script (includes model setup)"
	@echo "  make install-bin        - Install binary to ~/.local/bin (includes model setup)"
	@echo "  make clean              - Remove bin/ directory"
	@echo ""

start-engine:
	@bash scripts/voxtral-server.sh start

stop-engine:
	@bash scripts/voxtral-server.sh stop

status-engine:
	@bash scripts/voxtral-server.sh status

build:
	@echo "🔨 Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@cd cmd/local-whisper && go build -o ../../$(BINARY_PATH)
	@echo "✅ Built: ./$(BINARY_PATH)"

build-voice-monitor:
	@echo "🔨 Building $(MONITOR_BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@cd cmd/voice-monitor && go build -o ../../$(MONITOR_BINARY_PATH)
	@echo "✅ Built: ./$(MONITOR_BINARY_PATH)"

test:
	@echo "🧪 Running tests..."
	@go test -v ./...
	@cd scripts/voice_hooks && uv run pytest
	@echo "✅ Tests passed"

vet:
	@echo "🔍 Vetting Go code..."
	@go vet ./...
	@echo "✅ Vet passed"

fmt:
	@echo "🎨 Formatting Go code..."
	@gofmt -w .
	@echo "🎨 Formatting Python code (mlx-engine, voxtral, scripts/voice_hooks)..."
	@cd mlx-engine && uv run ruff format .
	@cd voxtral && uv run ruff format .
	@cd scripts/voice_hooks && uv run ruff format .
	@echo "✅ Formatted"

fmt-check:
	@echo "🎨 Checking Go formatting..."
	@test -z "$$(gofmt -l .)" || (echo "❌ Not gofmt'd:" && gofmt -l . && exit 1)
	@echo "🎨 Checking Python formatting (mlx-engine, voxtral, scripts/voice_hooks)..."
	@cd mlx-engine && uv run ruff format --check .
	@cd voxtral && uv run ruff format --check .
	@cd scripts/voice_hooks && uv run ruff format --check .
	@echo "✅ Formatting clean"

lint: vet fmt-check
	@echo "🔍 Linting Python code (mlx-engine, voxtral, scripts/voice_hooks)..."
	@cd mlx-engine && uv run ruff check .
	@cd voxtral && uv run ruff check .
	@cd scripts/voice_hooks && uv run ruff check .
	@echo "✅ Lint passed"

setup-deps:
	@bash scripts/setup-deps.sh

setup-model:
	@bash scripts/setup-model.sh

setup: setup-deps setup-model
	@echo ""
	@echo "✅ All setup complete! Ready to build and run."

setup-voxtral:
	@bash scripts/setup-voxtral.sh

setup-voice-hooks:
	@bash scripts/setup-voice-hooks.sh

setup-blackhole:
	@bash scripts/setup-blackhole.sh

install-raycast: build setup-model
	@echo "📦 Installing Raycast commands..."
	@mkdir -p $(RAYCAST_DIR)

	@echo "-> Creating whisper-transcribe.sh"
	@echo "#!/bin/bash" > $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "# @raycast.schemaVersion 1" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "# @raycast.title Dictate with Whisper" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "# @raycast.description Voice transcription using local Whisper model (CPU)" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "# @raycast.mode fullOutput" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "# @raycast.currentDirectoryPath" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "# @raycast.icon 🎙️" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "# @raycast.packageName Voice" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "# @raycast.author iksnerd" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "# Local whisper voice transcription" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "cd \"\$${RAYCAST_CURRENT_DIRECTORY_PATH:-$(abspath .)}\" || exit 1" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "exec $(abspath $(BINARY_PATH)) --engine=whisper" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@chmod +x $(RAYCAST_DIR)/whisper-transcribe.sh

	@echo "-> Creating voxtral-transcribe.sh"
	@echo "#!/bin/bash" > $(RAYCAST_DIR)/voxtral-transcribe.sh
	@echo "" >> $(RAYCAST_DIR)/voxtral-transcribe.sh
	@echo "# @raycast.schemaVersion 1" >> $(RAYCAST_DIR)/voxtral-transcribe.sh
	@echo "# @raycast.title Dictate with Voxtral" >> $(RAYCAST_DIR)/voxtral-transcribe.sh
	@echo "# @raycast.description Fast voice transcription using Voxtral MLX model (Apple Silicon)" >> $(RAYCAST_DIR)/voxtral-transcribe.sh
	@echo "# @raycast.mode fullOutput" >> $(RAYCAST_DIR)/voxtral-transcribe.sh
	@echo "# @raycast.currentDirectoryPath" >> $(RAYCAST_DIR)/voxtral-transcribe.sh
	@echo "# @raycast.icon 🧠" >> $(RAYCAST_DIR)/voxtral-transcribe.sh
	@echo "# @raycast.packageName Voice" >> $(RAYCAST_DIR)/voxtral-transcribe.sh
	@echo "# @raycast.author iksnerd" >> $(RAYCAST_DIR)/voxtral-transcribe.sh
	@echo "" >> $(RAYCAST_DIR)/voxtral-transcribe.sh
	@echo "cd \"$(abspath .)\" || exit 1" >> $(RAYCAST_DIR)/voxtral-transcribe.sh
	@echo "bash scripts/voxtral-server.sh start" >> $(RAYCAST_DIR)/voxtral-transcribe.sh
	@echo "exec $(abspath $(BINARY_PATH)) --engine=voxtral" >> $(RAYCAST_DIR)/voxtral-transcribe.sh
	@chmod +x $(RAYCAST_DIR)/voxtral-transcribe.sh

	@echo "-> Creating voxtral-toggle.sh"
	@echo "#!/bin/bash" > $(RAYCAST_DIR)/voxtral-toggle.sh
	@echo "" >> $(RAYCAST_DIR)/voxtral-toggle.sh
	@echo "# @raycast.schemaVersion 1" >> $(RAYCAST_DIR)/voxtral-toggle.sh
	@echo "# @raycast.title Toggle Voxtral Server" >> $(RAYCAST_DIR)/voxtral-toggle.sh
	@echo "# @raycast.description Start or Stop the Voxtral MLX memory-resident server" >> $(RAYCAST_DIR)/voxtral-toggle.sh
	@echo "# @raycast.mode compact" >> $(RAYCAST_DIR)/voxtral-toggle.sh
	@echo "# @raycast.icon ⚙️" >> $(RAYCAST_DIR)/voxtral-toggle.sh
	@echo "# @raycast.packageName Voice" >> $(RAYCAST_DIR)/voxtral-toggle.sh
	@echo "# @raycast.author iksnerd" >> $(RAYCAST_DIR)/voxtral-toggle.sh
	@echo "" >> $(RAYCAST_DIR)/voxtral-toggle.sh
	@echo "cd \"$(abspath .)\" || exit 1" >> $(RAYCAST_DIR)/voxtral-toggle.sh
	@echo "if [ -f /tmp/voxtral-server.pid ] && kill -0 \$$(cat /tmp/voxtral-server.pid) 2>/dev/null; then" >> $(RAYCAST_DIR)/voxtral-toggle.sh
	@echo "  bash scripts/voxtral-server.sh stop > /dev/null && echo '🛑 Voxtral Server Stopped'" >> $(RAYCAST_DIR)/voxtral-toggle.sh
	@echo "else" >> $(RAYCAST_DIR)/voxtral-toggle.sh
	@echo "  bash scripts/voxtral-server.sh start > /dev/null && echo '🚀 Voxtral Server Started'" >> $(RAYCAST_DIR)/voxtral-toggle.sh
	@echo "fi" >> $(RAYCAST_DIR)/voxtral-toggle.sh
	@chmod +x $(RAYCAST_DIR)/voxtral-toggle.sh

	@echo "✅ Installed all scripts to: $(RAYCAST_DIR)"
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
