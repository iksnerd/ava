package recording

import (
	"os/exec"

	"local-whisper/internal/audio"
	"local-whisper/internal/procutil"
)

// Recorder handles audio recording with silence detection
type Recorder struct {
	OutputPath string
	PlaySound  bool
}

// NewRecorder creates a new audio recorder
func NewRecorder(outputPath string, playSound bool) *Recorder {
	return &Recorder{
		OutputPath: outputPath,
		PlaySound:  playSound,
	}
}

// Record starts recording with sox
// - Starts recording immediately (skip initial silence)
// - Stops after 2.0s of silence at 3% threshold
func (r *Recorder) Record() error {
	// Play start sound in background
	if r.PlaySound {
		go func() {
			cmd := exec.Command("afplay", "/System/Library/Sounds/Blow.aiff")
			// Ensure audio can play by not redirecting stdout/stderr
			cmd.Stdout = nil
			cmd.Stderr = nil
			cmd.Run()
		}()
	}

	// Record with sox
	cmd := exec.Command("sox", "-d", "-r", audio.SampleRateHz, "-c", audio.Channels, r.OutputPath,
		"silence", "1", "0.01", "0.1%", "1", "2.0", "3%")

	// Suppress sox output
	closeSilence, err := procutil.Silence(cmd)
	if err != nil {
		return err
	}
	defer closeSilence()

	return cmd.Run()
}
