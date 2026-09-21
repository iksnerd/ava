package recording

import (
	"os/exec"

	"github.com/iksnerd/local-whisper/internal/audio"
	"github.com/iksnerd/local-whisper/internal/procutil"
	"github.com/iksnerd/local-whisper/internal/ttscontrol"
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
// - Peak-normalizes to -3dB before writing OutputPath
//
// Capture and normalization run as one sox effects chain rather than two
// separate invocations: the rate/channel conversion a later resample step
// would do is already a no-op here (sox captures directly at
// audio.SampleRateHz/Channels), so the only effect worth a second process
// was "norm -3" — folding it in here saves a full sox spawn and an
// intermediate temp file on every recording.
func (r *Recorder) Record() error {
	// Stop any in-flight Claude Voice TTS before opening the mic: otherwise
	// whatever it's currently speaking (e.g. a hook notification that
	// overlaps a dictation) goes out the speakers and back in through the
	// mic while sox is capturing.
	ttscontrol.StopSpeaking()

	// Play the start cue and let it finish before sox starts listening. sox
	// has no lead-in (it starts capturing immediately, not after the first
	// sound), so backgrounding this would let the beep itself bleed into
	// the recording through the mic.
	if r.PlaySound {
		cmd := exec.Command("afplay", "/System/Library/Sounds/Blow.aiff")
		cmd.Stdout = nil
		cmd.Stderr = nil
		cmd.Run()
	}

	// Record with sox
	cmd := exec.Command("sox", "-d", "-r", audio.SampleRateHz, "-c", audio.Channels, r.OutputPath,
		"silence", "1", "0.01", "0.1%", "1", "2.0", "3%", "norm", "-3")

	// Suppress sox output
	closeSilence, err := procutil.Silence(cmd)
	if err != nil {
		return err
	}
	defer closeSilence()

	return cmd.Run()
}
