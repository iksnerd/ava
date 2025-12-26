package whisper

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// TranscribeOptions contains transcription parameters
type TranscribeOptions struct {
	AudioPath     string
	OutputPath    string
	ContextPrompt string
	Language      string
}

// Transcribe transcribes audio using whisper-cli
func (c *Client) Transcribe(opts TranscribeOptions) (string, error) {
	// Check if model exists
	if _, err := os.Stat(c.ModelPath); err != nil {
		modelFile := filepath.Base(c.ModelPath)
		return "", fmt.Errorf("whisper model not found at %s. Download with: wget -O %s https://huggingface.co/ggerganov/whisper.cpp/resolve/main/%s", 
			c.ModelPath, c.ModelPath, modelFile)
	}

	// Transcribe with language parameter
	cmd := exec.Command("whisper-cli",
		"-m", c.ModelPath,
		"-f", opts.AudioPath,
		"-otxt", "-of", strings.TrimSuffix(opts.OutputPath, ".txt"),
		"-t", "8", "-nt", "-sns",
		"-l", opts.Language,
		"--prompt", opts.ContextPrompt,
	)

	// Suppress whisper-cli verbose output
	devNull, _ := os.Open(os.DevNull)
	cmd.Stderr = devNull
	cmd.Stdout = devNull

	err := cmd.Run()
	devNull.Close()
	if err != nil {
		return "", err
	}

	// Read transcribed text
	text, err := os.ReadFile(opts.OutputPath)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(text)), nil
}
