.PHONY: build test test-mlx-engine test-voxtral install-hooks uninstall-hooks vet fmt fmt-check lint check-paths check-names check-docs check-protocol generate-protocol check-enginedist generate-enginedist check-swift-config release-snapshot release-notes install-raycast install-bin uninstall setup-deps setup-model setup setup-voxtral setup-voice-hooks setup-blackhole start-engine stop-engine status-engine clean help

BINARY_NAME=ava
BUILD_DIR=bin
RAYCAST_DIR=$(HOME)/raycast-scripts
INSTALL_BIN_DIR=$(HOME)/.local/bin
# git describe so a build can name the exact commit it came from; a tarball
# with no .git falls back to the tag-less form, and `go install` gets its
# version from the module proxy instead (see internal/buildinfo).
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS=-ldflags "-X github.com/iksnerd/ava/internal/buildinfo.Version=$(VERSION)"
BINARY_PATH=$(BUILD_DIR)/$(BINARY_NAME)

help:
	@echo "ava Makefile"
	@echo ""
	@echo "Commands:"
	@echo "  make setup              - setup-deps + setup-model: everything dictation and Kokoro need"
	@echo "  make check-swift-config - Check VoiceSettings.swift resolves the config like bash and Go (needs swift)"
	@echo "  make check-paths        - Fail if any tracked file hardcodes a home directory"
	@echo "  make generate-protocol  - Regenerate the shared constants from internal/protocol/protocol.json"
	@echo "  make check-protocol     - Fail if a generated constants file is stale"
	@echo "  make generate-enginedist - Refresh the engine bundle embedded in the binary"
	@echo "  make check-enginedist   - Fail if the embedded engine bundle is stale"
	@echo "  make release-snapshot   - Build the release artifacts locally, publishing nothing"
	@echo "  make release-notes      - Print the newest CHANGELOG.md section (for --release-notes)"
	@echo "  make check-names        - Fail if a tracked filename breaks another checkout"
	@echo "  make check-docs         - Fail if a make target or script is documented nowhere"
	@echo "  make install-hooks      - Enable the pre-commit hook (CI only runs on tags)"
	@echo "  make setup-deps         - Install system dependencies (sox, whisper-cli, uv)"
	@echo "  make setup-model        - Download Whisper model to ~/.local/share/whisper-cpp/"
	@echo "  make setup-voxtral      - Python venv for ava monitor's realtime streaming (Voxtral)"
	@echo "  make setup-voice-hooks  - Create Python venv for the voice-hook text/summarization CLIs"
	@echo "  make setup-blackhole    - Install BlackHole loopback driver to capture system/call audio"
	@echo "  make build              - Build the binary to bin/"
	@echo "  make test               - Run all tests (Go, voice-hooks, mlx-engine, voxtral)"
	@echo "  make vet                - go vet the Go code"
	@echo "  make fmt                - Format Go (gofmt) and Python (ruff format), in place"
	@echo "  make fmt-check          - Check formatting without modifying files (CI-safe)"
	@echo "  make lint               - vet, fmt-check, the check-* guards (not check-swift-config) and ruff check"
	@echo "  make start-engine       - Start the mlx-engine Kokoro TTS server in the background"
	@echo "  make stop-engine        - Stop the mlx-engine Kokoro TTS server"
	@echo "  make status-engine      - Check if the mlx-engine Kokoro TTS server is running"
	@echo "  make install-raycast    - Install as Raycast command script (includes model setup)"
	@echo "  make install-bin        - Install binary to ~/.local/bin (includes model setup)"
	@echo "  make uninstall          - Remove the installed binaries, Raycast script and engine bundle"
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
	@cd cmd/ava && go build $(LDFLAGS) -o ../../$(BINARY_PATH)
	@echo "✅ Built: ./$(BINARY_PATH)"

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

check-names:
	@bash scripts/check-filenames.sh

generate-protocol:
	@go run ./internal/protocol/gen

check-protocol:
	@go run ./internal/protocol/gen -check

generate-enginedist:
	@go run ./internal/enginedist/gen

check-enginedist:
	@go run ./internal/enginedist/gen -check

# Build exactly what a tag would publish, without publishing it. The --clean
# is what makes the check meaningful: a stale dist/ from an earlier run is
# indistinguishable from a successful build of the current tree.
release-snapshot:
	@goreleaser release --snapshot --clean

# The newest CHANGELOG.md section, for `goreleaser release --release-notes`.
# The changelog is written by hand and is the release notes; GoReleaser's own
# commit-list generator is disabled so the two cannot contradict each other.
release-notes:
	@awk '/^## /{n++} n==1' CHANGELOG.md | tail -n +2

check-docs:
	@bash scripts/check-doc-coverage.sh

# CI only runs on version tags, so this hook is the pre-merge signal. It checks
# the staged snapshot rather than the working tree — see .githooks/pre-commit.
install-hooks:
	@git config core.hooksPath .githooks
	@echo "✅ core.hooksPath -> .githooks (pre-commit active)"
	@echo "   Skip a run with: git commit --no-verify"

uninstall-hooks:
	@git config --unset core.hooksPath || true
	@echo "✅ hooks disabled"

lint: vet fmt-check check-paths check-names check-protocol check-enginedist check-docs
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
	@echo "# @raycast.title Dictate with Ava" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "# @raycast.description Dictate into the focused app, on-device" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "# @raycast.mode fullOutput" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "# @raycast.currentDirectoryPath" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "# @raycast.icon 🎙️" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "# @raycast.packageName Voice" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "# @raycast.author iksnerd" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "cd \"\$${RAYCAST_CURRENT_DIRECTORY_PATH:-$(abspath .)}\" || exit 1" >> $(RAYCAST_DIR)/whisper-transcribe.sh
	@echo "exec $(abspath $(BINARY_PATH))" >> $(RAYCAST_DIR)/whisper-transcribe.sh
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
	@# install, not cp: cp rewrites the file in place, and macOS kills a running
	@# process (a long-lived `ava mcp`) whose binary changes under it.
	@install -m 0755 $(BINARY_PATH) $(INSTALL_BIN_DIR)/$(BINARY_NAME)
	@echo "✅ Installed: $(INSTALL_BIN_DIR)/$(BINARY_NAME)"
	@echo ""
	@echo "Setup complete! You can now use:"
	@echo "  $(BINARY_NAME)"
	@echo ""
	@echo "Optional: Add to your PATH in ~/.zshrc or ~/.bash_profile:"
	@echo "  export PATH=\"$(INSTALL_BIN_DIR):\$$PATH\""
# Only what nothing else could be using. The whisper models, the Hugging Face
# cache (shared with every MLX tool), Ava.app and its config are left alone and
# listed instead; docs/uninstall.md covers them. --keep-autostart so stopping
# the server does not rewrite the config the menu bar app may still be using.
ENGINE_DIR=$(HOME)/Library/Application Support/ava/engine
uninstall:
	@if [ -x "$(INSTALL_BIN_DIR)/$(BINARY_NAME)" ]; then \
		"$(INSTALL_BIN_DIR)/$(BINARY_NAME)" engine stop --keep-autostart >/dev/null 2>&1 || true; \
	fi
	@for f in "$(INSTALL_BIN_DIR)/$(BINARY_NAME)" \
		"$(RAYCAST_DIR)/whisper-transcribe.sh" "$(ENGINE_DIR)"; do \
		if [ -e "$$f" ] || [ -L "$$f" ]; then rm -rf "$$f" && echo "🗑️  Removed $$f"; fi; \
	done
	@echo "✅ Uninstalled. Left in place, see docs/uninstall.md to remove them:"
	@echo "   ~/.local/share/whisper-cpp/ (whisper models)"
	@echo "   ~/.cache/huggingface/hub/models--mlx-community--* (Kokoro, Voxtral; shared cache)"
	@echo "   /Applications/Ava.app and ~/Library/Application Support/ava/config.json"
	@echo "   Voice hook and MCP entries in ~/.claude/settings.json, if you added them"
	@echo "   Microphone and Accessibility grants in System Settings"

clean:
	@echo "🧹 Cleaning up..."
	@rm -rf $(BUILD_DIR)
	@echo "✅ Cleaned"
