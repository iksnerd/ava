package whisper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/iksnerd/ava/internal/procutil"
	"github.com/iksnerd/ava/pkg/stt"
	"github.com/iksnerd/ava/pkg/stt/captions"
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

	args := buildArgs(c.ModelPath, opts)
	if !opts.WantsTimestamps() {
		out, err := c.run(args)
		if err != nil {
			return "", err
		}
		text := strings.TrimSpace(string(out))
		stt.WriteOutputIfRequested(opts, text)
		return text, nil
	}

	rendered, err := c.captions(args, opts)
	if err != nil {
		return "", err
	}
	stt.WriteOutputIfRequested(opts, rendered)
	return rendered, nil
}

// run executes whisper-cli with args under the client's timeout and returns
// its stdout. The one place a run is started, so plain text and captions
// cannot differ in how they time out or report a failure.
func (c *Client) run(args []string) ([]byte, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "whisper-cli", args...)
	// Killing whisper-cli is not enough if something it started still holds
	// the output pipe (a wrapper script's child): Output() would wait on the
	// pipe regardless. WaitDelay bounds that wait after the kill.
	cmd.WaitDelay = 2 * time.Second
	// ... and whisper-cli's own children: a timeout kills the whole process
	// group, not only the program, and nothing it started is left running.
	procutil.InOwnGroup(cmd)
	procutil.KillGroupOnCancel(cmd)
	// cmd.Output() leaves Stderr nil, so it buffers the child's stderr
	// (whisper-cli's verbose model-load/timing logs) into ExitError.Stderr
	// instead of printing it — silencing it on success, and giving a real
	// diagnostic instead of a bare "exit status 1" on failure.

	out, err := cmd.Output()
	procutil.ReapGroup(cmd)
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("whisper-cli timed out after %s", timeout)
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("whisper-cli failed: %w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("whisper-cli failed: %w", err)
	}
	return out, nil
}

// captions runs whisper-cli for its JSON, which carries each segment's start
// and end in milliseconds, and renders the requested format from that. An
// empty result means no speech; a run that exits 0 and writes no JSON is an
// error, because it would otherwise read as silence.
func (c *Client) captions(args []string, opts stt.Options) (string, error) {
	dir, err := os.MkdirTemp("", "ava-whisper-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir for whisper-cli output: %w", err)
	}
	defer os.RemoveAll(dir)
	base := filepath.Join(dir, "out")

	if _, err := c.run(withJSONOutput(args, base)); err != nil {
		return "", err
	}
	raw, err := os.ReadFile(base + ".json")
	if err != nil {
		return "", fmt.Errorf("whisper-cli exited cleanly but wrote no timestamps (%s.json): %w", base, err)
	}
	var parsed struct {
		Transcription []struct {
			Offsets struct {
				From int `json:"from"`
				To   int `json:"to"`
			} `json:"offsets"`
			Text string `json:"text"`
		} `json:"transcription"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("read whisper-cli timestamps: %w", err)
	}
	segments := make([]captions.Segment, len(parsed.Transcription))
	for i, s := range parsed.Transcription {
		segments[i] = captions.Segment{StartMS: s.Offsets.From, EndMS: s.Offsets.To, Text: s.Text}
	}

	switch opts.Format {
	case "srt":
		return captions.SRT(segments), nil
	case "vtt":
		return captions.VTT(segments), nil
	default:
		return captions.JSON(segments)
	}
}

// withJSONOutput swaps -nt (no timestamps on stdout) for -oj -of base, which
// makes whisper-cli write base.json with the timed segments instead.
func withJSONOutput(args []string, base string) []string {
	out := make([]string, 0, len(args)+2)
	for _, a := range args {
		if a == "-nt" {
			out = append(out, "-oj", "-of", base)
			continue
		}
		out = append(out, a)
	}
	return out
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
