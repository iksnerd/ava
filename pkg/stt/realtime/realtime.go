// Package realtime wraps realtime.py, the Python/MLX realtime
// transcription primitive under voxtral/, the same way pkg/stt/whisper
// wraps whisper-cli. Used only by internal/monitor — a different,
// independent thing from pkg/mlx (the HTTP client for ava
// -engine voxtral): same underlying model family, different local
// architecture. Don't confuse the two.
package realtime

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Client shells out to voxtral/realtime.py.
type Client struct {
	PythonPath string // path to the venv's python3, e.g. voxtral/.venv/bin/python3 (see: uv sync in voxtral/)
	ScriptDir  string // path to the voxtral/ directory containing realtime.py
}

// NewClient creates a new realtime STT client.
func NewClient(pythonPath, scriptDir string) *Client {
	return &Client{
		PythonPath: pythonPath,
		ScriptDir:  scriptDir,
	}
}

// checkPython verifies the venv python from NewClient actually exists,
// since every entry point below shells out to it.
func (c *Client) checkPython() error {
	if _, err := os.Stat(c.PythonPath); err != nil {
		return fmt.Errorf("voxtral venv python not found at %s. Run: make setup-voxtral", c.PythonPath)
	}
	return nil
}

// RealtimeDelta is one transcript event emitted by realtime.py.
type RealtimeDelta struct {
	Event    string `json:"event"` // "ready", "delta", or "done"
	Text     string `json:"text"`
	Device   string `json:"device"`   // set on the "ready" event
	Engine   string `json:"engine"`   // set on the "ready" event: "voxtral" or "whisper"
	Language string `json:"language"` // set on the "ready" event for the whisper engine
	Speaker  string `json:"speaker"`  // set on "delta" events when diarization is enabled, e.g. "Speaker 0"
}

// RealtimeOptions configures StreamRealtime's input source and STT engine.
type RealtimeOptions struct {
	// Device selects an input device by name substring (e.g. "BlackHole",
	// to capture system/call audio via a loopback driver) or index. Empty
	// uses the system default microphone.
	Device string

	// Engine selects the STT engine: "voxtral" (default, <500ms latency,
	// 13 languages) or "whisper" (multilingual, 99+ languages incl.
	// Bulgarian, ~1s latency).
	Engine string

	// Model overrides the engine's default HF repo id.
	Model string

	// Language is a language code (e.g. "bg" for Bulgarian). Only used by
	// the whisper engine; it defaults to English if left empty.
	Language string

	// Diarize tags each delta with a speaker label via Sortformer
	// diarization. Whisper engine only. Can only ever distinguish remote
	// participants already mixed into the captured audio - never labels
	// this machine's own mic input.
	Diarize bool

	// HighpassHz is a high-pass filter cutoff in Hz applied before
	// STT/diarization, cutting low-frequency rumble/bass. Zero leaves this
	// flag unset, so realtime.py's own default (80Hz) applies.
	HighpassHz float64
}

// Stream is a running realtime.py session.
type Stream struct {
	cmd  *exec.Cmd
	done chan struct{}
	err  error
}

// Done is closed once the process has exited and every event it printed has
// been delivered, whether Stop asked it to or it died on its own.
func (s *Stream) Done() <-chan struct{} { return s.done }

// Err is the process's exit error. It is only meaningful once Done is closed.
func (s *Stream) Err() error { return s.err }

// Stop interrupts the process and returns its exit error once it has gone.
func (s *Stream) Stop() error {
	_ = s.cmd.Process.Signal(os.Interrupt)
	<-s.done
	return s.err
}

// StreamRealtime starts realtime.py listening on opts.Device (or the default
// microphone if empty) using opts.Engine. onDelta is called for every event
// on its stdout until Stop is called or the process exits on its own. Unlike
// Transcribe, this is a long-lived subprocess, not a one-shot call.
func (c *Client) StreamRealtime(opts RealtimeOptions, onDelta func(RealtimeDelta)) (*Stream, error) {
	if err := c.checkPython(); err != nil {
		return nil, err
	}

	args := []string{filepath.Join(c.ScriptDir, "realtime.py")}
	if opts.Device != "" {
		args = append(args, "--device", opts.Device)
	}
	if opts.Engine != "" {
		args = append(args, "--engine", opts.Engine)
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Language != "" {
		args = append(args, "--language", opts.Language)
	}
	if opts.Diarize {
		args = append(args, "--diarize")
	}
	if opts.HighpassHz != 0 {
		args = append(args, "--highpass-hz", strconv.FormatFloat(opts.HighpassHz, 'f', -1, 64))
	}

	cmd := exec.Command(c.PythonPath, args...)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// cmd.Wait() closes stdout as soon as the process exits, so it has to
	// come after the scan loop has drained the pipe: waiting first can drop
	// the last buffered events, such as the final "done".
	stream := &Stream{cmd: cmd, done: make(chan struct{})}
	go func() {
		defer close(stream.done)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			var delta RealtimeDelta
			if json.Unmarshal(scanner.Bytes(), &delta) == nil {
				onDelta(delta)
			}
		}
		stream.err = cmd.Wait()
	}()

	return stream, nil
}

// ListInputDevices runs realtime.py --list-devices and returns its stdout,
// e.g. to find a BlackHole loopback device name/index for RealtimeOptions.Device.
func (c *Client) ListInputDevices() (string, error) {
	if err := c.checkPython(); err != nil {
		return "", err
	}

	cmd := exec.Command(c.PythonPath, filepath.Join(c.ScriptDir, "realtime.py"), "--list-devices")
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("voxtral list-devices failed: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}
