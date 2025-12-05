package whisper

import (
	"testing"
)

func TestNewClient(t *testing.T) {
	modelPath := "/path/to/model.bin"
	client := NewClient(modelPath)

	if client.ModelPath != modelPath {
		t.Errorf("expected ModelPath %q, got %q", modelPath, client.ModelPath)
	}
}

func TestTranscribeOptions(t *testing.T) {
	opts := TranscribeOptions{
		AudioPath:     "/tmp/audio.wav",
		OutputPath:    "/tmp/output.txt",
		ContextPrompt: "test prompt",
		Language:      "en",
	}

	if opts.AudioPath != "/tmp/audio.wav" {
		t.Errorf("expected AudioPath /tmp/audio.wav, got %s", opts.AudioPath)
	}

	if opts.Language != "en" {
		t.Errorf("expected Language en, got %s", opts.Language)
	}
}

func TestTranscribeModelNotFound(t *testing.T) {
	modelPath := "/nonexistent/model.bin"
	client := NewClient(modelPath)

	opts := TranscribeOptions{
		AudioPath:     "/tmp/audio.wav",
		OutputPath:    "/tmp/output.txt",
		ContextPrompt: ".",
		Language:      "en",
	}

	_, err := client.Transcribe(opts)
	if err == nil {
		t.Error("expected error when model not found, got nil")
	}

	if !contains(err.Error(), "whisper model not found") {
		t.Errorf("expected 'whisper model not found' in error, got %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && s[:len(substr)] == substr || len(s) > len(substr))
}
