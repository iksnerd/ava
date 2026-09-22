package whisper

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/iksnerd/ava/pkg/stt"
)

// DefaultTimeout bounds one transcription. base.en runs at roughly 16x real
// time, so this covers files of several hours; it exists so that a hung
// whisper-cli cannot block its caller (an MCP transcribe call, say) forever.
const DefaultTimeout = 30 * time.Minute

// Client wraps the whisper-cli command-line tool
type Client struct {
	ModelPath string
	Timeout   time.Duration
}

// NewClient creates a new Whisper client
func NewClient(modelPath string) *Client {
	return &Client{
		ModelPath: modelPath,
		Timeout:   DefaultTimeout,
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
		fix := "ava setup-model"
		if strings.Contains(filepath.Base(c.ModelPath), "tiny") {
			fix += " --model tiny"
		}
		return "", fmt.Errorf("whisper model not found at %s.\n"+
			"Download it with:\n"+
			"  %s", c.ModelPath, fix)
	}

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "whisper-cli", buildArgs(c.ModelPath, opts)...)
	// Killing whisper-cli is not enough if something it started still holds
	// the output pipe (a wrapper script's child): Output() would wait on the
	// pipe regardless. WaitDelay bounds that wait after the kill.
	cmd.WaitDelay = 2 * time.Second
	// cmd.Output() leaves Stderr nil, so it buffers the child's stderr
	// (whisper-cli's verbose model-load/timing logs) into ExitError.Stderr
	// instead of printing it — silencing it on success, and giving a real
	// diagnostic instead of a bare "exit status 1" on failure.

	out, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("whisper-cli timed out after %s", timeout)
	}
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

// buildArgs is whisper-cli's argv. Optional decoding flags are only passed
// when set, so an unset one keeps whatever default the installed whisper-cli
// has rather than a copy of it frozen here.
func buildArgs(modelPath string, opts stt.Options) []string {
	args := []string{
		"-m", modelPath,
		"-f", opts.AudioPath,
		"-t", "8", "-nt", "-sns",
		"-l", opts.Language,
		"--prompt", opts.ContextPrompt,
	}
	if opts.BeamSize > 0 {
		args = append(args, "-bs", strconv.Itoa(opts.BeamSize))
	}
	return args
}
