package recording

import (
	"os"
	"os/exec"
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
			cmd.Run()
		}()
	}

	// Record with sox
	cmd := exec.Command("sox", "-d", "-r", "16000", "-c", "1", r.OutputPath,
		"silence", "1", "0.01", "0.1%", "1", "2.0", "3%")

	// Show sox output for debugging silence detection
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	return cmd.Run()
}
