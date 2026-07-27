// Package voxtral wraps the Python/MLX primitive scripts under voxtral/
// (stt.py, realtime.py, tts.py), the same way pkg/whisper wraps whisper-cli.
package voxtral

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

// Client shells out to the Voxtral MLX primitives.
type Client struct {
	PythonPath string // path to the venv's python3, e.g. voxtral/.venv/bin/python3 (see: uv sync in voxtral/)
	ScriptDir  string // path to the voxtral/ directory containing stt.py, realtime.py, tts.py
}

// NewClient creates a new Voxtral client.
func NewClient(pythonPath, scriptDir string) *Client {
	return &Client{
		PythonPath: pythonPath,
		ScriptDir:  scriptDir,
	}
}

// TranscribeOptions configures a one-shot Voxtral Mini 3B transcription.
type TranscribeOptions struct {
	AudioPath string
	Language  string
}

// Transcribe runs Voxtral Mini 3B (stt.py) on a recorded audio file.
func (c *Client) Transcribe(opts TranscribeOptions) (string, error) {
	if _, err := os.Stat(c.PythonPath); err != nil {
		return "", fmt.Errorf("voxtral venv python not found at %s. Run: make setup-voxtral", c.PythonPath)
	}

	cmd := exec.Command(c.PythonPath, filepath.Join(c.ScriptDir, "stt.py"),
		"--audio", opts.AudioPath,
		"--language", opts.Language,
	)
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("voxtral stt failed: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}

// SpeakOptions configures Voxtral TTS synthesis.
type SpeakOptions struct {
	Text       string
	Voice      string
	OutputPath string
}

// Speak runs Voxtral TTS (tts.py), writing a wav file to opts.OutputPath.
func (c *Client) Speak(opts SpeakOptions) error {
	if _, err := os.Stat(c.PythonPath); err != nil {
		return fmt.Errorf("voxtral venv python not found at %s. Run: make setup-voxtral", c.PythonPath)
	}

	voice := opts.Voice
	if voice == "" {
		voice = "casual_male"
	}

	cmd := exec.Command(c.PythonPath, filepath.Join(c.ScriptDir, "tts.py"),
		"--text", opts.Text,
		"--voice", voice,
		"--output", opts.OutputPath,
	)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("voxtral tts failed: %w", err)
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

// StreamRealtime starts realtime.py listening on opts.Device (or the default
// microphone if empty) using opts.Engine. onDelta is called for every event
// on its stdout until the returned stop function is invoked or the process
// exits on its own. Unlike Transcribe, this is a long-lived subprocess, not
// a one-shot call.
func (c *Client) StreamRealtime(opts RealtimeOptions, onDelta func(RealtimeDelta)) (stop func() error, err error) {
	if _, err := os.Stat(c.PythonPath); err != nil {
		return nil, fmt.Errorf("voxtral venv python not found at %s. Run: make setup-voxtral", c.PythonPath)
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

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			var delta RealtimeDelta
			if json.Unmarshal(scanner.Bytes(), &delta) == nil {
				onDelta(delta)
			}
		}
	}()

	stop = func() error {
		if cmd.Process != nil {
			cmd.Process.Signal(os.Interrupt)
		}
		return cmd.Wait()
	}

	return stop, nil
}

// ListInputDevices runs realtime.py --list-devices and returns its stdout,
// e.g. to find a BlackHole loopback device name/index for RealtimeOptions.Device.
func (c *Client) ListInputDevices() (string, error) {
	if _, err := os.Stat(c.PythonPath); err != nil {
		return "", fmt.Errorf("voxtral venv python not found at %s. Run: make setup-voxtral", c.PythonPath)
	}

	cmd := exec.Command(c.PythonPath, filepath.Join(c.ScriptDir, "realtime.py"), "--list-devices")
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("voxtral list-devices failed: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}
