package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadContextPrompt(t *testing.T) {
	dir := t.TempDir()
	contextFile := filepath.Join(dir, "context.txt")
	globalFile := filepath.Join(dir, "global.txt")
	writeFile(t, contextFile, "project context")
	writeFile(t, globalFile, "global context")
	missing := filepath.Join(dir, "missing.txt")

	t.Run("explicit context file wins", func(t *testing.T) {
		prompt, status := loadContextPrompt(contextFile, globalFile)
		if !strings.HasPrefix(prompt, "project context ") {
			t.Errorf("prompt = %q, want prefix %q", prompt, "project context ")
		}
		if !strings.Contains(status, contextFile) {
			t.Errorf("status = %q, want it to mention %q", status, contextFile)
		}
	})

	t.Run("falls back to global context", func(t *testing.T) {
		prompt, status := loadContextPrompt("", globalFile)
		if !strings.HasPrefix(prompt, "global context ") {
			t.Errorf("prompt = %q, want prefix %q", prompt, "global context ")
		}
		if !strings.Contains(status, "Global context loaded") {
			t.Errorf("status = %q, want it to mention global context", status)
		}
	})

	t.Run("missing explicit context file does not fall back", func(t *testing.T) {
		prompt, status := loadContextPrompt(missing, globalFile)
		if prompt != noContextPrompt {
			t.Errorf("prompt = %q, want bare %q (no silent fallback to global)", prompt, noContextPrompt)
		}
		if status != "" {
			t.Errorf("status = %q, want empty", status)
		}
	})

	t.Run("nothing configured", func(t *testing.T) {
		prompt, status := loadContextPrompt("", "")
		if prompt != noContextPrompt {
			t.Errorf("prompt = %q, want bare %q", prompt, noContextPrompt)
		}
		if status != "" {
			t.Errorf("status = %q, want empty", status)
		}
	})

	t.Run("missing global context is silently skipped", func(t *testing.T) {
		prompt, status := loadContextPrompt("", missing)
		if prompt != noContextPrompt || status != "" {
			t.Errorf("loadContextPrompt with missing global file = (%q, %q), want (%q, \"\")", prompt, status, noContextPrompt)
		}
	})
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
}
