package recording

import (
	"errors"
	"os/exec"
	"sync"
	"syscall"

	"github.com/iksnerd/ava/internal/audio"
	"github.com/iksnerd/ava/internal/procutil"
	"github.com/iksnerd/ava/internal/ttscontrol"
)

// Recorder handles audio recording with silence detection
type Recorder struct {
	OutputPath string
	PlaySound  bool

	mu      sync.Mutex
	cmd     *exec.Cmd // the running sox, nil before start and after exit
	stopped bool      // Stop or Kill was called; sox must not (re)start
}

// ErrStoppedBeforeStart is returned by Record when Stop or Kill arrived before
// sox was started (during the start cue, say): nothing was recorded, and it
// was deliberate, so it is not a microphone problem.
var ErrStoppedBeforeStart = errors.New("recording stopped before it started")

// Stop ends the recording the way the terminal's Ctrl+C does: it sends sox
// SIGINT, which makes sox finalize the WAV header and exit, so Record returns
// with the audio captured so far on disk. Safe to call from a signal-handling
// goroutine, at any time, any number of times. When the terminal already sent
// the same SIGINT to the whole foreground group, sox just sees it twice.
func (r *Recorder) Stop() { r.signal(syscall.SIGINT) }

// Kill ends the recording without finalizing: sox is killed, the mic closes,
// and the file is not usable. For a cancel, never for a stop.
func (r *Recorder) Kill() { r.signal(syscall.SIGKILL) }

func (r *Recorder) signal(sig syscall.Signal) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopped = true
	if r.cmd != nil && r.cmd.Process != nil {
		_ = r.cmd.Process.Signal(sig)
	}
}

// NewRecorder creates a new audio recorder
func NewRecorder(outputPath string, playSound bool) *Recorder {
	return &Recorder{
		OutputPath: outputPath,
		PlaySound:  playSound,
	}
}

// Record starts recording with sox
//   - Starts recording immediately (skip initial silence)
//   - Stops after 2.0s of silence at 3% threshold, or when Stop is called
//     (the dictation's Ctrl+C), keeping what was captured
//   - Peak-normalizes to -3dB before writing OutputPath
//
// Capture and normalization run as one sox effects chain rather than two
// separate invocations: the rate/channel conversion a later resample step
// would do is already a no-op here (sox captures directly at
// audio.SampleRateHz/Channels), so the only effect worth a second process
// was "norm -3" — folding it in here saves a full sox spawn and an
// intermediate temp file on every recording.
func (r *Recorder) Record() error {
	// Stop any in-flight Ava TTS before opening the mic: otherwise
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

	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return ErrStoppedBeforeStart
	}
	if err := cmd.Start(); err != nil {
		r.mu.Unlock()
		return err
	}
	r.cmd = cmd
	r.mu.Unlock()

	err = cmd.Wait()

	r.mu.Lock()
	r.cmd = nil
	stopped := r.stopped
	r.mu.Unlock()

	// sox exits on SIGINT with a non-zero status or a signal death on some
	// builds even though it finalized the file. A requested stop is not a
	// failure; the caller judges the file by its size.
	if err != nil && stopped {
		return nil
	}
	return err
}
