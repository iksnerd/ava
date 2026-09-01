package whisper

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"local-whisper/pkg/stt"
)

// Client wraps the whisper-cli command-line tool
type Client struct {
	ModelPath string
}

// NewClient creates a new Whisper client
func NewClient(modelPath string) *Client {
	return &Client{
		ModelPath: modelPath,
	}
}

// Transcribe transcribes audio using whisper-cli. With -nt (no timestamps)
// and no -otxt/-of, whisper-cli's stdout is exactly the transcript, so it's
// captured directly rather than round-tripped through a file on disk.
func (c *Client) Transcribe(opts stt.Options) (string, error) {
	// Check if model exists
	if _, err := os.Stat(c.ModelPath); err != nil {
		modelFile := filepath.Base(c.ModelPath)
		return "", fmt.Errorf("whisper model not found at %s. Download with: wget -O %s https://huggingface.co/ggerganov/whisper.cpp/resolve/main/%s",
			c.ModelPath, c.ModelPath, modelFile)
	}

	cmd := exec.Command("whisper-cli",
		"-m", c.ModelPath,
		"-f", opts.AudioPath,
		"-t", "8", "-nt", "-sns",
		"-l", opts.Language,
		"--prompt", opts.ContextPrompt,
	)
	// cmd.Output() leaves Stderr nil, so it buffers the child's stderr
	// (whisper-cli's verbose model-load/timing logs) into ExitError.Stderr
	// instead of printing it — silencing it on success, and giving a real
	// diagnostic instead of a bare "exit status 1" on failure.

	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return "", fmt.Errorf("whisper-cli failed: %w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("whisper-cli failed: %w", err)
	}

	text := strings.TrimSpace(string(out))

	// Writing OutputPath is for debugging/caching; a failure here doesn't
	// invalidate an otherwise-successful transcription, so it's reported
	// rather than returned as an error (matching pkg/stt/mlx).
	if opts.OutputPath != "" {
		if err := os.WriteFile(opts.OutputPath, []byte(text), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️ Failed to write output file %s: %v\n", opts.OutputPath, err)
		}
	}

	return text, nil
}
