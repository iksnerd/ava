package whisper

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/iksnerd/local-whisper/pkg/stt"
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
		// This is the first error anyone who installed the binary without the
		// repo sees, so it names the command that fixes it rather than a
		// make target they have no checkout to run.
		fix := "local-whisper setup-model"
		if strings.Contains(filepath.Base(c.ModelPath), "tiny") {
			fix += " --model tiny"
		}
		return "", fmt.Errorf("whisper model not found at %s.\n"+
			"Download it with:\n"+
			"  %s", c.ModelPath, fix)
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
	stt.WriteOutputIfRequested(opts, text)
	return text, nil
}
