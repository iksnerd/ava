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
// sidecar after the request returns. That is how another process stops a
// speak; the context passed to Speak is how the caller's own process does,
// and it does not replace the marker files.
//
// The protocol's constants are not spelled here: they live in
// internal/ttsproto, generated from internal/protocol/protocol.json with the
// bash and Swift copies that scripts/stop-speaking.sh and the menu bar read.
package speaker

import (
	"context"
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
//
// Every method takes the speak's context: ending it aborts the request in
// flight, and an implementation returns an error that wraps ctx.Err().
type Engine interface {
	Check(ctx context.Context) mlx.State
	Speak(ctx context.Context, opts mlx.SpeakOptions) ([]byte, error)
	SpeakTimed(ctx context.Context, opts mlx.SpeakOptions) (mlx.TimedSpeech, error)
}

// ErrStaleEngine (match it with errors.Is) is an engine that answers /health
// without speaking this ava's protocol, almost always one left running across
// an upgrade. It is not auto-restarted: another session may be mid-synthesis
// on it.
var ErrStaleEngine = errors.New("the running mlx-engine is older than this ava; " +
	"restart it with `ava engine stop && ava engine start`")

// Options are per-call overrides. A zero value takes everything from the
// live voice settings, which is what a caller with no opinion wants.
type Options struct {
	// Voice is a Kokoro voice id, or several comma-separated to blend.
	Voice string
	// Speed is Kokoro's multiplier; 0 means "use the configured speed".
	Speed float64
	// Async returns as soon as the work is handed off, as a hook needs. Errors then go to stderr, since there's no caller
	// left to return them to. The handed-off work no longer follows the
	// caller's context: an MCP request ends the moment it returns, and the
	// speech it asked for must outlive that. Stop it with the marker-file
	// protocol (internal/ttscontrol) instead.
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
	// OutDir speaks each non-blank line of the text into its own WAV in this
	// directory, with a manifest. See renderBatch. It excludes Out and Timings.
	OutDir string
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
	Engine    Engine
	Settings  func() voiceconfig.Settings
	Play      func(ctx context.Context, audioPath string, volume float64, pidFile string) error
	Say       func(ctx context.Context, text string, rate int, outPath string) error
	AutoStart func(ctx context.Context) error
	// StartGuard stops auto-start being retried when it keeps failing. Nil means
	// no limit, which is what a test wants.
	StartGuard  *startGuard
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
		Engine:    mlx.NewClient(serverURL),
		Settings:  voiceconfig.Load,
		AutoStart: startEngine,
		StartGuard: &startGuard{
			Path:   filepath.Join(os.TempDir(), "ava-engine-start-failures"),
			Max:    startFailuresMax,
			Window: startFailuresWindow,
		},
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
//
// Ending ctx abandons the speak: the synthesis request is aborted, a wait for
// the playback lock ends, the player is killed, the marker and its sidecars
// are removed, and the error wraps ctx.Err(). It is not a failure of the
// engine, so it never falls back to `say` and never starts the engine. A ctx
// that is already over does nothing at all. This is the in-process way to
// stop; stopping from another process is still the marker-file protocol.
func (s *Speaker) Speak(ctx context.Context, text string, opts Options) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if opts.OutDir != "" {
		return s.renderBatch(ctx, text, opts)
	}
	if opts.Timings != "" && opts.Out == "" {
		return errors.New("timings are written beside a WAV: pass out as well")
	}
	if opts.Out != "" {
		return s.render(ctx, text, opts)
	}
	if s.Settings().Muted {
		return nil
	}
	if opts.Async {
		detached := context.WithoutCancel(ctx)
		go func() {
			if err := s.speak(detached, text, opts); err != nil {
				fmt.Fprintf(os.Stderr, "speak: %v\n", err)
			}
		}()
		return nil
	}
	return s.speak(ctx, text, opts)
}

func (s *Speaker) speak(ctx context.Context, text string, opts Options) error {
	settings := s.Settings()

	marker, err := s.markActive()
	if err != nil {
		return err
	}
	defer clearMarker(marker)

	audio, synthErr := s.synthesize(ctx, text, opts, settings)
	if err := ctx.Err(); err != nil {
		// The caller walked away. That is not the engine failing, so it is
		// not a reason to speak in a different voice.
		return err
	}
	if stopped(marker) {
		// A deliberate stop, not a failure — the user asked for silence,
		// not for the same sentence in a different voice.
		return nil
	}
	if synthErr == nil {
		return s.playTemp(ctx, audio, "ava-tts-*.wav", settings.Volume, marker)
	}

	var limited *StartLimitedError
	switch {
	case errors.Is(synthErr, ErrStaleEngine):
		s.notef("%v — speaking through macOS `say` instead, "+
			"so this will not use your configured Kokoro voice.", synthErr)
	case errors.As(synthErr, &limited):
		s.notef("%v — speaking through macOS `say` instead. "+
			"Run `ava engine start` to see why it fails.", synthErr)
	default:
		s.notef("mlx-engine unavailable (%v) — speaking through macOS `say` instead, "+
			"so this will not use your configured Kokoro voice. "+
			"Start it with `ava engine start`.", synthErr)
	}
	return s.speakWithSay(ctx, text, settings, marker)
}

// render synthesizes text into opts.Out, and opts.Timings when asked. Files
// are only created once the engine has answered, so a failure leaves nothing
// behind to be mistaken for the narration, and a timings file never exists
// without the WAV it describes.
func (s *Speaker) render(ctx context.Context, text string, opts Options) error {
	settings := s.Settings()
	if err := s.ready(ctx, settings); err != nil {
		return fmt.Errorf("cannot write %s: %w", opts.Out, err)
	}
	req := s.request(text, opts, settings)

	if opts.Timings == "" {
		audio, err := s.Engine.Speak(ctx, req)
		if err != nil {
			return fmt.Errorf("cannot write %s: %w", opts.Out, err)
		}
		return writeFile(opts.Out, audio)
	}

	timed, err := s.Engine.SpeakTimed(ctx, req)
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
func (s *Speaker) synthesize(ctx context.Context, text string, opts Options, settings voiceconfig.Settings) ([]byte, error) {
	if err := s.ready(ctx, settings); err != nil {
		return nil, err
	}
	return s.Engine.Speak(ctx, s.request(text, opts, settings))
}

// ready is nil once there is an engine on our protocol to talk to, starting
// one if it is down and the user has not stopped it on purpose. A stale
// engine is not started over; see ErrStaleEngine.
func (s *Speaker) ready(ctx context.Context, settings voiceconfig.Settings) error {
	switch s.Engine.Check(ctx) {
	case mlx.Stale:
		return ErrStaleEngine
	case mlx.Ready:
		s.StartGuard.Succeeded() // up, however it got there: earlier failures no longer count
		return nil
	}
	// A cancelled health check reads as Down. Do not take that for an engine
	// that needs starting: a start costs a `uv sync` and counts against the limit.
	if err := ctx.Err(); err != nil {
		return err
	}

	if !settings.EngineAutoStart || s.AutoStart == nil {
		return fmt.Errorf("mlx-engine is not running")
	}
	if until, blocked := s.StartGuard.Blocked(); blocked {
		return &StartLimitedError{Until: until, Max: s.StartGuard.Max}
	}
	if err := s.AutoStart(ctx); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr // abandoned, not failed: it does not count
		}
		s.StartGuard.Failed()
		return fmt.Errorf("mlx-engine auto-start failed: %w", err)
	}
	switch s.Engine.Check(ctx) {
	case mlx.Stale:
		return ErrStaleEngine
	case mlx.Down:
		if err := ctx.Err(); err != nil {
			return err
		}
		s.StartGuard.Failed()
		return fmt.Errorf("mlx-engine did not come up")
	}
	s.StartGuard.Succeeded()
	return nil
}

// StartLimitedError (match it with errors.As) is an engine that has failed to
// start too often to try again for now (RFC 0004 section 6, restart intensity). Retrying costs a `uv sync`
// and up to two minutes of waiting on every speak, hooks included.
type StartLimitedError struct {
	// Until is when the oldest counted failure ages out and a start is tried again.
	Until time.Time
	// Max is how many failures inside the window cause the limit.
	Max int
}

func (e *StartLimitedError) Error() string {
	return fmt.Sprintf("mlx-engine failed to start %d times in a row; not trying again until %s",
		e.Max, e.Until.Format("15:04"))
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
func (s *Speaker) speakWithSay(ctx context.Context, text string, settings voiceconfig.Settings, marker string) error {
	out, err := os.CreateTemp("", "ava-tts-say-*.aiff")
	if err != nil {
		return fmt.Errorf("create temp audio file: %w", err)
	}
	path := out.Name()
	out.Close()
	defer os.Remove(path)

	if err := s.Say(ctx, text, settings.SayRate, path); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("say fallback failed: %w", err)
	}
	if stopped(marker) {
		return nil
	}
	return s.Play(ctx, path, settings.Volume, marker+".play.pid")
}

func (s *Speaker) playTemp(ctx context.Context, audio []byte, pattern string, volume float64, marker string) error {
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

	return s.Play(ctx, path, volume, marker+".play.pid")
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
func (s *Speaker) playLocked(ctx context.Context, audioPath string, volume float64, pidFile string) error {
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
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(lockPollInterval):
		}
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	// A stop between the last poll above and the PID file below finds no
	// player to kill, so check on both sides of that gap.
	if stopped(marker) {
		return nil
	}
	// Likewise a cancel that arrived while the lock was being taken.
	if err := ctx.Err(); err != nil {
		return err
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

	timeout := time.NewTimer(playbackTimeout)
	defer timeout.Stop()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-done
		return ctx.Err()
	case <-timeout.C:
		_ = cmd.Process.Kill()
		<-done
		return fmt.Errorf("afplay exceeded %s and was killed", playbackTimeout)
	}
}

func writePid(pidFile string, pid int) {
	_ = os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0644)
}

func renderWithSay(ctx context.Context, text string, rate int, outPath string) error {
	return exec.CommandContext(ctx, "say", "-r", strconv.Itoa(rate), "-o", outPath, text).Run()
}

// startEngine shells out to the same control script `ava engine
// start` uses, rather than reimplementing its PID-file locking and uvicorn
// management here.
func startEngine(ctx context.Context) error {
	out, err := exec.CommandContext(ctx, "bash", engineScript(), "start").CombinedOutput()
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
