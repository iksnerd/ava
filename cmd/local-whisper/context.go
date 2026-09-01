package main

import (
	"fmt"
	"os"
)

// noContextPrompt is the whisper-cli initial prompt used when no context
// file is loaded.
const noContextPrompt = ". "

// loadContextPrompt resolves the transcription context prompt: an explicit
// --context file takes priority, then the per-user global context file
// (globalContextPath, normally ~/.whisper-context), falling back to no
// context. status is a human-readable line for --verbose output, empty if
// nothing was loaded.
func loadContextPrompt(contextFile, globalContextPath string) (prompt, status string) {
	if contextFile != "" {
		if content, err := os.ReadFile(contextFile); err == nil {
			return string(content) + " " + noContextPrompt, fmt.Sprintf("📂 Context loaded: %s", contextFile)
		}
		return noContextPrompt, ""
	}
	if globalContextPath != "" {
		if content, err := os.ReadFile(globalContextPath); err == nil {
			return string(content) + " " + noContextPrompt, "🌍 Global context loaded: ~/.whisper-context"
		}
	}
	return noContextPrompt, ""
}
