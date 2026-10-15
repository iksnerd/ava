// Package speaker speaks text through the local Kokoro TTS server, falling
// back to macOS's `say` when that server isn't available.
//
// It is the one speak implementation: `ava speak`, the MCP server, and
// scripts/speak.sh (the hooks and the menu bar app), which hands off to
// `ava speak`. It used to be one of two, and a stop fix made here missed
// the bash copy. Everything watching speech sees the same protocol:
//
//   - the global mute in ~/Library/Application Support/ava/config.json
//     is checked before anything is synthesized (internal/voiceconfig);
//   - an activity marker file exists in /tmp/ava-tts-active for the whole
//     synth+playback window, which AvaMenuBar's SpeechActivityMonitor
//     polls for its speaking indicator and internal/ttscontrol looks for when
//     cancelling;
//   - playback holds an exclusive flock(2) on /tmp/ava-tts-playback.lock,
//     so concurrent speaks from several processes queue instead of talking
//     over one another.
//
// Synthesis has no PID sidecar: it is an in-process HTTP call, and naming
// our own PID would have a stop kill the whole server.
// Cancellation mid-synthesis is instead observed through the .stopped
// sidecar after the request returns.
//
// The protocol's constants are not spelled here: they live in
// internal/ttsproto, generated from internal/protocol/protocol.json with the
// bash and Swift copies that scripts/stop-speaking.sh and the menu bar read.
package speaker

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/iksnerd/ava/internal/enginedist"
	"github.com/iksnerd/ava/internal/ttsproto"
	"github.com/iksnerd/ava/internal/voiceconfig"
	"github.com/iksnerd/ava/pkg/mlx"
	"github.com/iksnerd/ava/pkg/stt/captions"
)

// playbackTimeout is long enough for a
// whole article read aloud in one go, short enough that one wedged player
// can't block every other session's audio forever.
const playbackTimeout = 600 * time.Second

// EngineScriptEnv overrides where the mlx-engine control script lives, for
// an installed binary that isn't running from the repo root. Same escape
// hatch `ava engine --script` offers.
const EngineScriptEnv = enginedist.ScriptEnv

// Engine is the TTS half of the mlx-engine client, narrowed to what
// speaking needs (satisfied by *mlx.Client).
type Engine interface {
	Check() mlx.State
	Speak(mlx.SpeakOptions) ([]byte, error)
	SpeakTimed(mlx.SpeakOptions) (mlx.TimedSpeech, error)
}

// errStaleEngine is an engine that answers /health without speaking this
// ava's protocol, almost always one left running across an upgrade. It is
// not auto-restarted: another session may be mid-synthesis on it.
var errStaleEngine = errors.New("the running mlx-engine is older than this ava; " +
	"restart it with `ava engine stop && ava engine start`")

// Options are per-call overrides. A zero value takes everything from the
// live voice settings, which is what a caller with no opinion wants.
type Options struct {
	// Voice is a Kokoro voice id, or several comma-separated to blend.
	Voice string
	// Speed is Kokoro's multiplier; 0 means "use the configured speed".
	Speed float64
	// Async returns as soon as the work is handed off, as a hook needs. Errors then go to stderr, since there's no caller
	// left to return them to.
	Async bool
	// Out writes the engine's WAV to this path instead of playing it. Nothing
	// is spoken, so the mute, the activity marker and the playback lock do not
	// apply, and there is no `say` fallback: a different voice in a file a
	// script will mux is worse than an error.
	Out string
	// Timings, with Out, also writes where each chunk of the text sits in the
	// audio, as JSON, from the same synthesis. Captions need it.
	Timings string
	// SplitPattern is Kokoro's chunking regex and so how fine Timings' segments
	// are; see SplitPattern for the named presets. Empty keeps the engine's
	// default, a segment per line.
	SplitPattern string
}

// sentenceSplit breaks after sentence-ending punctuation, so each segment of
// a timed render is one sentence.
const sentenceSplit = `(?<=[.!?])\s+`

// SplitPattern turns a preset name into the regex the engine takes: "line"
// (or empty) is the engine's own default, "sentence" a segment per sentence.
// The CLI and the MCP tool both resolve their name through it.
func SplitPattern(name string) (string, error) {
	switch name {
	case "", "line":
		return "", nil
	case "sentence":
		return sentenceSplit, nil
	}
	return "", fmt.Errorf("unknown split %q (choose line or sentence)", name)
}

// Speaker performs synthesis and playback. The function fields are the
// process-spawning seams, injected so tests can exercise the whole
// mute/marker/fallback protocol without playing audio.
type Speaker struct {
	Engine      Engine
	Settings    func() voiceconfig.Settings
	Play        func(audioPath string, volume float64, pidFile string) error
	Say         func(text string, rate int, outPath string) error
	AutoStart   func() error
	ActivityDir string
	LockPath    string
	// Notice receives user-facing warnings that are not failures — above all,
	// that synthesis fell back to macOS `say`. Without it the caller hears a
	// completely different voice, in noticeably worse quality, and is told
	// nothing: the most common "why does it sound robotic / why is it the wrong
	// voice" confusion. nil means discard, which is what a test wants.
	Notice io.Writer
}

// New wires a Speaker against the real mlx-engine, afplay and `say`.
// serverURL may be empty for the default 127.0.0.1:8765.
func New(serverURL string) *Speaker {
	s := &Speaker{
		Engine:      mlx.NewClient(serverURL),
		Settings:    voiceconfig.Load,
		AutoStart:   startEngine,
		ActivityDir: ttsproto.ActivityDir(),
		LockPath:    ttsproto.LockPath(),
	}
	s.Play = s.playLocked
	s.Say = renderWithSay
	s.Notice = os.Stderr
	return s
}

// notef writes a non-fatal warning where the user will see it, if anywhere.
func (s *Speaker) notef(format string, args ...any) {
	if s.Notice == nil {
		return
	}
	fmt.Fprintf(s.Notice, "⚠️  "+format+"\n", args...)
}

// Speak says text out loud and returns once it has finished playing (unless
// opts.Async). A muted machine is not an error: it returns nil having done
// nothing, so no caller has to special-case silence.
func (s *Speaker) Speak(text string, opts Options) error {
	if opts.Timings != "" && opts.Out == "" {
		return errors.New("timings are written beside a WAV: pass out as well")
	}
	if opts.Out != "" {
		return s.render(text, opts)
	}
	if s.Settings().Muted {
		return nil
	}
	if opts.Async {
		go func() {
			if err := s.speak(text, opts); err != nil {
				fmt.Fprintf(os.Stderr, "speak: %v\n", err)
			}
		}()
		return nil
	}
	return s.speak(text, opts)
}

func (s *Speaker) speak(text string, opts Options) error {
	settings := s.Settings()

	marker, err := s.markActive()
	if err != nil {
		return err
	}
	defer clearMarker(marker)

	audio, synthErr := s.synthesize(text, opts, settings)
	if stopped(marker) {
		// A deliberate stop, not a failure — the user asked for silence,
		// not for the same sentence in a different voice.
		return nil
	}
	if synthErr == nil {
		return s.playTemp(audio, "ava-tts-*.wav", settings.Volume, marker)
	}

	if errors.Is(synthErr, errStaleEngine) {
		s.notef("%v — speaking through macOS `say` instead, "+
			"so this will not use your configured Kokoro voice.", synthErr)
	} else {
		s.notef("mlx-engine unavailable (%v) — speaking through macOS `say` instead, "+
			"so this will not use your configured Kokoro voice. "+
			"Start it with `ava engine start`.", synthErr)
	}
	return s.speakWithSay(text, settings, marker)
}

// render synthesizes text into opts.Out, and opts.Timings when asked. Files
// are only created once the engine has answered, so a failure leaves nothing
// behind to be mistaken for the narration, and a timings file never exists
// without the WAV it describes.
func (s *Speaker) render(text string, opts Options) error {
	settings := s.Settings()
	if err := s.ready(settings); err != nil {
		return fmt.Errorf("cannot write %s: %w", opts.Out, err)
	}
	req := s.request(text, opts, settings)

	if opts.Timings == "" {
		audio, err := s.Engine.Speak(req)
		if err != nil {
			return fmt.Errorf("cannot write %s: %w", opts.Out, err)
		}
		return writeFile(opts.Out, audio)
	}

	timed, err := s.Engine.SpeakTimed(req)
	if err != nil {
		return fmt.Errorf("cannot write %s: %w", opts.Out, err)
	}
	segments, err := captions.JSON(timed.Segments)
	if err != nil {
		return err
	}
	if err := writeFile(opts.Out, timed.Audio); err != nil {
		return err
	}
	if err := writeFile(opts.Timings, []byte(segments)); err != nil {
		_ = os.Remove(opts.Out)
		return err
	}
	return nil
}

func writeFile(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// synthesize returns the WAV bytes from the engine, starting the server
// first if it's down and the user hasn't explicitly stopped it. An error
// here is the caller's cue to fall back to `say`.
func (s *Speaker) synthesize(text string, opts Options, settings voiceconfig.Settings) ([]byte, error) {
	if err := s.ready(settings); err != nil {
		return nil, err
	}
	return s.Engine.Speak(s.request(text, opts, settings))
}

// ready is nil once there is an engine on our protocol to talk to, starting
// one if it is down and the user has not stopped it on purpose. A stale
// engine is not started over; see errStaleEngine.
func (s *Speaker) ready(settings voiceconfig.Settings) error {
	switch s.Engine.Check() {
	case mlx.Stale:
		return errStaleEngine
	case mlx.Down:
		if !settings.EngineAutoStart || s.AutoStart == nil {
			return fmt.Errorf("mlx-engine is not running")
		}
		if err := s.AutoStart(); err != nil {
			return fmt.Errorf("mlx-engine auto-start failed: %w", err)
		}
		switch s.Engine.Check() {
		case mlx.Stale:
			return errStaleEngine
		case mlx.Down:
			return fmt.Errorf("mlx-engine did not come up")
		}
	}
	return nil
}

// request is what the engine is asked for: the caller's voice and speed, or
// the configured ones.
func (s *Speaker) request(text string, opts Options, settings voiceconfig.Settings) mlx.SpeakOptions {
	voice := opts.Voice
	if voice == "" {
		voice = settings.Voice
	}
	speed := opts.Speed
	if speed == 0 {
		speed = settings.Speed
	}
	return mlx.SpeakOptions{Text: text, Voice: voice, Speed: speed, SplitPattern: opts.SplitPattern}
}

// speakWithSay is the always-available fallback. `say` has no volume flag,
// so render to a file and play it through afplay too — that keeps the
// volume slider meaningful even with the server down.
func (s *Speaker) speakWithSay(text string, settings voiceconfig.Settings, marker string) error {
	out, err := os.CreateTemp("", "ava-tts-say-*.aiff")
	if err != nil {
		return fmt.Errorf("create temp audio file: %w", err)
	}
	path := out.Name()
	out.Close()
	defer os.Remove(path)

	if err := s.Say(text, settings.SayRate, path); err != nil {
		return fmt.Errorf("say fallback failed: %w", err)
	}
	if stopped(marker) {
		return nil
	}
	return s.Play(path, settings.Volume, marker+".play.pid")
}

func (s *Speaker) playTemp(audio []byte, pattern string, volume float64, marker string) error {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return fmt.Errorf("create temp audio file: %w", err)
	}
	path := f.Name()
	if _, err := f.Write(audio); err != nil {
		f.Close()
		os.Remove(path)
		return fmt.Errorf("write temp audio file: %w", err)
	}
	f.Close()
	defer os.Remove(path)

	return s.Play(path, volume, marker+".play.pid")
}

// markActive creates this speak's marker file. The name only has to be
// unique — ttscontrol treats any non-sidecar file in the directory as a
// marker — so it carries a timestamp as well as the PID, since one process
// can have several speaks in flight at once (the MCP server).
func (s *Speaker) markActive() (string, error) {
	if err := os.MkdirAll(s.ActivityDir, 0755); err != nil {
		return "", fmt.Errorf("create activity dir: %w", err)
	}
	marker := filepath.Join(s.ActivityDir, fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano()))
	if err := os.WriteFile(marker, nil, 0644); err != nil {
		return "", fmt.Errorf("create activity marker: %w", err)
	}
	return marker, nil
}

func clearMarker(marker string) {
	for _, path := range []string{marker, marker + ".play.pid", marker + ".stopped"} {
		_ = os.Remove(path)
	}
}

// stopped reports whether ttscontrol.StopSpeaking() (or stop-speaking.sh)
// cancelled this speak while it was in flight.
func stopped(marker string) bool {
	_, err := os.Stat(marker + ".stopped")
	return err == nil
}

// lockPollInterval is how often a queued speak retries the playback lock
// and checks whether it has been stopped.
const lockPollInterval = 50 * time.Millisecond

// playLocked runs afplay under an exclusive flock so concurrent speaks —
// another Claude Code session's hook, a manual Read Aloud — queue instead of
// overlapping.
//
// The PID file only ever names the player. It used to name this process
// while it waited for the lock, so that a stop could cancel a queued speak;
// but every stop SIGTERMs that PID, and this process is `ava` itself — for
// `ava mcp`, the MCP server. A queued speak is cancelled instead by polling
// for the .stopped sidecar, which every stop writes before it signals.
func (s *Speaker) playLocked(audioPath string, volume float64, pidFile string) error {
	lock, err := os.OpenFile(s.LockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("open playback lock: %w", err)
	}
	defer lock.Close()

	marker := strings.TrimSuffix(pidFile, ttsproto.PlayPIDSuffix)
	for {
		err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return fmt.Errorf("acquire playback lock: %w", err)
		}
		if stopped(marker) {
			return nil
		}
		time.Sleep(lockPollInterval)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	// A stop between the last poll above and the PID file below finds no
	// player to kill, so check on both sides of that gap.
	if stopped(marker) {
		return nil
	}

	cmd := exec.Command("afplay", "-v", strconv.FormatFloat(volume, 'f', -1, 64), audioPath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start afplay: %w", err)
	}
	writePid(pidFile, cmd.Process.Pid)
	defer os.Remove(pidFile)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	if stopped(marker) {
		_ = cmd.Process.Kill()
		<-done
		return nil
	}

	select {
	case err := <-done:
		return err
	case <-time.After(playbackTimeout):
		_ = cmd.Process.Kill()
		<-done
		return fmt.Errorf("afplay exceeded %s and was killed", playbackTimeout)
	}
}

func writePid(pidFile string, pid int) {
	_ = os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0644)
}

func renderWithSay(text string, rate int, outPath string) error {
	return exec.Command("say", "-r", strconv.Itoa(rate), "-o", outPath, text).Run()
}

// startEngine shells out to the same control script `ava engine
// start` uses, rather than reimplementing its PID-file locking and uvicorn
// management here.
func startEngine() error {
	out, err := exec.Command("bash", engineScript(), "start").CombinedOutput()
	if err != nil {
		// The script's last lines say why (no engine found, a crash during
		// startup); without them the user saw only "exit status 1".
		if reason := lastLines(string(out), 3); reason != "" {
			return fmt.Errorf("%w: %s", err, reason)
		}
	}
	return err
}

// lastLines returns the final n non-empty lines of s, joined with "; ".
func lastLines(s string, n int) string {
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "; ")
}

// engineScript is the control script auto-start runs: the same one
// `ava engine` would, from enginedist.ResolveScript. With none found it
// returns the checkout-relative path, so the failure names what was missing.
func engineScript() string {
	if script, err := enginedist.ResolveScript(""); err == nil {
		return script
	}
	return enginedist.ScriptRelPath
}
