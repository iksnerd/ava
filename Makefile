.PHONY: build build-voice-monitor test test-mlx-engine test-voxtral vet fmt fmt-check lint check-paths check-swift-config install-raycast install-bin setup-deps setup-model setup setup-voxtral setup-voice-hooks setup-blackhole clean help

BINARY_NAME=local-whisper
MONITOR_BINARY_NAME=voice-monitor
BUILD_DIR=bin
RAYCAST_DIR=$(HOME)/raycast-scripts
INSTALL_BIN_DIR=$(HOME)/.local/bin
# git describe so a build can name the exact commit it came from; a tarball
# with no .git falls back to the tag-less form, and `go install` gets its
# version from the module proxy instead (see internal/buildinfo).
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS=-ldflags "-X github.com/iksnerd/local-whisper/internal/buildinfo.Version=$(VERSION)"
BINARY_PATH=$(BUILD_DIR)/$(BINARY_NAME)
MONITOR_BINARY_PATH=$(BUILD_DIR)/$(MONITOR_BINARY_NAME)

help:
	@echo "local-whisper Makefile"
	@echo ""
	@echo "Commands:"
	@echo "  make setup              - Install all dependencies (sox, whisper-cli) and download model"
	@echo "  make check-swift-config - Check VoiceSettings.swift resolves the config like bash and Go (needs swift)"
	@echo "  make check-paths        - Fail if any tracked file hardcodes a home directory"
	@echo "  make setup-deps         - Install system dependencies (sox, whisper-cli, uv)"
	@echo "  make setup-model        - Download Whisper model to ~/.local/share/whisper-cpp/"
	@echo "  make setup-voxtral      - Python venv for voice-monitor's realtime streaming (Voxtral)"
	@echo "  make setup-voice-hooks  - Create Python venv for the voice-hook text/summarization CLIs"
	@echo "  make setup-blackhole    - Install BlackHole loopback driver to capture system/call audio"
	@echo "  make build              - Build the binary to bin/"
	@echo "  make build-voice-monitor - Build the realtime transcript monitor (localhost + log file)"
	@echo "  make test               - Run all tests (Go, voice-hooks, mlx-engine, voxtral)"
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

start-engine: build
	@$(BINARY_PATH) engine start

stop-engine: build
	@$(BINARY_PATH) engine stop

status-engine: build
	@$(BINARY_PATH) engine status

build:
	@echo "🔨 Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@cd cmd/local-whisper && go build $(LDFLAGS) -o ../../$(BINARY_PATH)
	@echo "✅ Built: ./$(BINARY_PATH)"

build-voice-monitor:
	@echo "🔨 Building $(MONITOR_BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@cd cmd/voice-monitor && go build $(LDFLAGS) -o ../../$(MONITOR_BINARY_PATH)
	@echo "✅ Built: ./$(MONITOR_BINARY_PATH)"

test:
	@echo "🧪 Running tests..."
	@go test -v ./...
	@cd scripts/voice_hooks && uv run pytest
	@$(MAKE) --no-print-directory test-mlx-engine
	@$(MAKE) --no-print-directory test-voxtral
	@echo "✅ Tests passed"

# mlx-engine's HTTP contract, stubbing MLX so this needs neither Apple Silicon
# nor the 146-package torch/CUDA lock. Deliberately NOT `cd mlx-engine && uv run
# pytest`: that resolves the whole environment for tests that never touch a
# model. See mlx-engine/tests/conftest.py.
test-mlx-engine:
	@echo "🧪 mlx-engine HTTP contract..."
	@cd mlx-engine && uvx --with fastapi --with httpx --with pytest --with python-multipart \
		pytest tests/ -q -p no:warnings

# realtime.py's pure parts — the high-pass filter and --device resolution — with
# sounddevice stubbed so no audio hardware is needed. scipy and numpy are real:
# the filter is what is under test.
test-voxtral:
	@echo "🧪 voxtral filter + device resolution..."
	@cd voxtral && uvx --with numpy --with scipy --with pytest \
		pytest tests/ -q -p no:warnings

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

check-swift-config:
	@echo "🔍 Checking VoiceSettings.swift against the config contract..."
	@python3 AvaMenuBar/scripts/check-config-contract.py

check-paths:
	@bash scripts/check-portable-paths.sh

lint: vet fmt-check check-paths
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
