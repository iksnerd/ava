package speaker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/iksnerd/ava/internal/testutil"
	"github.com/iksnerd/ava/internal/ttsproto"
	"github.com/iksnerd/ava/internal/voiceconfig"
	"github.com/iksnerd/ava/pkg/mlx"
)

// blockingEngine is ready, then hangs in Speak until its ctx ends, as a real
// HTTP request does against an engine that is mid-synthesis.
type blockingEngine struct {
	entered chan struct{}
	checks  int
}

func newBlockingEngine() *blockingEngine { return &blockingEngine{entered: make(chan struct{}, 4)} }

func (e *blockingEngine) Check(ctx context.Context) mlx.State { e.checks++; return mlx.Ready }
func (e *blockingEngine) Speak(ctx context.Context, _ mlx.SpeakOptions) ([]byte, error) {
	e.entered <- struct{}{}
	<-ctx.Done()
	return nil, ctx.Err()
}
func (e *blockingEngine) SpeakTimed(ctx context.Context, _ mlx.SpeakOptions) (mlx.TimedSpeech, error) {
	e.entered <- struct{}{}
	<-ctx.Done()
	return mlx.TimedSpeech{}, ctx.Err()
}

// within fails the test if fn does not return in d, and returns its result.
func within(t *testing.T, d time.Duration, what string, fn func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(d):
		t.Fatalf("%s did not return within %s of its context ending", what, d)
		return nil
	}
}

func waitFor(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// dirEntries lists names left in dir, for "leaves no marker or sidecar".
func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestCancelDuringSynthesisReturnsPromptlyAndLeavesNothing(t *testing.T) {
	engine := newBlockingEngine()
	s, rec := newTestSpeaker(t, engine, voiceconfig.Defaults())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { waitFor(t, engine.entered, "synthesis to start"); cancel() }()
	err := within(t, time.Second, "Speak", func() error { return s.Speak(ctx, "hello", Options{}) })

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Speak() error = %v, want context.Canceled", err)
	}
	if rec.saidText != "" {
		t.Errorf("fell back to say (%q) after the caller cancelled", rec.saidText)
	}
	if len(rec.played) != 0 {
		t.Errorf("played %v after cancel", rec.played)
	}
	if left := dirEntries(t, s.ActivityDir); len(left) != 0 {
		t.Errorf("activity dir still holds %v after cancel", left)
	}
}

// The write paths take the same ctx: a cancelled render must not be reported
// as a missing engine, and must not write the file.
func TestCancelDuringRenderToFileReturnsCtxErr(t *testing.T) {
	engine := newBlockingEngine()
	s, _ := newTestSpeaker(t, engine, voiceconfig.Defaults())
	out := filepath.Join(t.TempDir(), "x.wav")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { waitFor(t, engine.entered, "synthesis to start"); cancel() }()
	err := within(t, time.Second, "Speak --out", func() error { return s.Speak(ctx, "hello", Options{Out: out}) })

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Speak(Out) error = %v, want context.Canceled", err)
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Error("a cancelled render left its WAV behind")
	}
}

func TestCancelDuringBatchRenderReturnsCtxErr(t *testing.T) {
	engine := newBlockingEngine()
	s, _ := newTestSpeaker(t, engine, voiceconfig.Defaults())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := filepath.Join(t.TempDir(), "scenes")

	go func() { waitFor(t, engine.entered, "synthesis to start"); cancel() }()
	err := within(t, time.Second, "Speak --out-dir", func() error {
		return s.Speak(ctx, "one\ntwo", Options{OutDir: dir})
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Speak(OutDir) error = %v, want context.Canceled", err)
	}
}

// A context that is already over means the caller no longer wants the speech:
// nothing is checked, started, synthesized or said.
func TestAlreadyCancelledContextDoesNothing(t *testing.T) {
	engine := &stubEngine{healthy: false, audio: []byte("wav")}
	started := false
	s, rec := newTestSpeaker(t, engine, voiceconfig.Defaults())
	s.AutoStart = func(context.Context) error { started = true; return nil }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	for name, opts := range map[string]Options{
		"speak": {}, "render": {Out: filepath.Join(t.TempDir(), "x.wav")},
		"batch": {OutDir: filepath.Join(t.TempDir(), "d")}, "async": {Async: true},
	} {
		if err := s.Speak(ctx, "hello", opts); !errors.Is(err, context.Canceled) {
			t.Errorf("%s: Speak() error = %v, want context.Canceled", name, err)
		}
	}
	if started {
		t.Error("the engine was started for a cancelled speak")
	}
	if len(engine.calls) != 0 || len(engine.timedCalls) != 0 {
		t.Error("the engine was asked to synthesize for a cancelled speak")
	}
	if rec.saidText != "" || len(rec.played) != 0 {
		t.Errorf("said %q / played %v for a cancelled speak", rec.saidText, rec.played)
	}
	if left := dirEntries(t, s.ActivityDir); len(left) != 0 {
		t.Errorf("activity dir holds %v", left)
	}
}

// Cancelling while the engine is down must not start it: starting costs a
// uv sync and counts against the restart limit.
func TestCancelBeforeAutoStartDoesNotStartTheEngine(t *testing.T) {
	engine := &stubEngine{healthy: false}
	started := false
	s, rec := newTestSpeaker(t, engine, voiceconfig.Defaults())
	ctx, cancel := context.WithCancel(context.Background())
	engine.onCheck = cancel // the cancel lands during the health check
	s.AutoStart = func(context.Context) error { started = true; return nil }

	err := s.Speak(ctx, "hello", Options{})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Speak() error = %v, want context.Canceled", err)
	}
	if started {
		t.Error("auto-start ran after the caller cancelled")
	}
	if rec.saidText != "" {
		t.Errorf("fell back to say (%q) after the caller cancelled", rec.saidText)
	}
}

// A cancel during auto-start is not a failed start: it must not count toward
// the restart limit, and must not fall back to say.
func TestCancelDuringAutoStartIsNotAFailedStart(t *testing.T) {
	engine := &stubEngine{healthy: false}
	s, rec := newTestSpeaker(t, engine, voiceconfig.Defaults())
	s.StartGuard = &startGuard{Path: filepath.Join(t.TempDir(), "failures"), Max: 1, Window: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.AutoStart = func(ctx context.Context) error {
		cancel()
		return ctx.Err()
	}

	err := s.Speak(ctx, "hello", Options{})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Speak() error = %v, want context.Canceled", err)
	}
	if _, blocked := s.StartGuard.Blocked(); blocked {
		t.Error("a cancelled start was counted as a failed start")
	}
	if rec.saidText != "" {
		t.Errorf("fell back to say (%q)", rec.saidText)
	}
}

// Async hands the work off and returns, so the caller's ctx ending right after
// (as an MCP request's does) must not kill the speech it asked for.
func TestAsyncSpeechOutlivesTheCallersContext(t *testing.T) {
	engine := &stubEngine{healthy: true, audio: []byte("wav")}
	s, rec := newTestSpeaker(t, engine, voiceconfig.Defaults())
	played := make(chan struct{})
	play := s.Play
	s.Play = func(ctx context.Context, p string, v float64, pid string) error {
		defer close(played)
		if err := ctx.Err(); err != nil {
			t.Errorf("async playback saw a dead context: %v", err)
		}
		return play(ctx, p, v, pid)
	}
	ctx, cancel := context.WithCancel(context.Background())

	if err := s.Speak(ctx, "hello", Options{Async: true}); err != nil {
		t.Fatalf("Speak(Async): %v", err)
	}
	cancel()
	select {
	case <-played:
	case <-time.After(2 * time.Second):
		t.Fatal("async speech never played after the caller's context ended")
	}
	if len(rec.played) != 1 {
		t.Errorf("played %v, want one clip", rec.played)
	}
}

// stubSlowAfplay is an afplay that publishes its PID and then plays for 30s.
func stubSlowAfplay(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	pidOut := filepath.Join(t.TempDir(), "afplay.pid")
	script := "#!/bin/sh\necho $$ > " + pidOut + "\nexec sleep 30\n"
	if err := os.WriteFile(filepath.Join(bin, "afplay"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	testutil.PrependPath(t, bin)
	return pidOut
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func TestCancelWhileQueuedBehindThePlaybackLockStopsWaiting(t *testing.T) {
	afplayPID := stubAfplay(t)
	s := &Speaker{LockPath: filepath.Join(t.TempDir(), "playback.lock")}
	marker := filepath.Join(t.TempDir(), "marker")
	release := holdLock(t, s.LockPath)
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { time.Sleep(100 * time.Millisecond); cancel() }()
	err := within(t, time.Second, "playLocked", func() error {
		return s.playLocked(ctx, "clip.wav", 0, marker+ttsproto.PlayPIDSuffix)
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("playLocked() error = %v, want context.Canceled", err)
	}
	time.Sleep(60 * time.Millisecond)
	if _, statErr := os.Stat(afplayPID); statErr == nil {
		t.Error("a speak cancelled while queued still started afplay")
	}
}

func TestCancelDuringPlaybackKillsThePlayer(t *testing.T) {
	afplayPID := stubSlowAfplay(t)
	s := &Speaker{LockPath: filepath.Join(t.TempDir(), "playback.lock")}
	pidFile := filepath.Join(t.TempDir(), "marker") + ttsproto.PlayPIDSuffix
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- s.playLocked(ctx, "clip.wav", 0, pidFile) }()

	var pid int
	for i := 0; i < 200 && pid == 0; i++ {
		time.Sleep(10 * time.Millisecond)
		if data, err := os.ReadFile(afplayPID); err == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		}
	}
	if pid == 0 || !alive(pid) {
		t.Fatalf("the stub player never ran (pid %d)", pid)
	}
	// The pid file is published after Start; wait for it so the cancel lands
	// mid-playback rather than in the gap.
	for i := 0; i < 200; i++ {
		if _, err := os.Stat(pidFile); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("playLocked() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("playback did not stop when its context was cancelled")
	}
	for i := 0; i < 100 && alive(pid); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if alive(pid) {
		t.Errorf("afplay (pid %d) still running after cancel", pid)
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	if _, err := os.Stat(pidFile); err == nil {
		t.Error("play pid file left behind after cancel")
	}
}

// End to end through Speak: a cancel during real playback leaves no marker,
// sidecar or player.
func TestCancelDuringPlaybackThroughSpeakLeavesNothing(t *testing.T) {
	afplayPID := stubSlowAfplay(t)
	engine := &stubEngine{healthy: true, audio: []byte("wav")}
	s, _ := newTestSpeaker(t, engine, voiceconfig.Defaults())
	s.Play = s.playLocked
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		for i := 0; i < 200; i++ {
			if _, err := os.Stat(afplayPID); err == nil {
				time.Sleep(50 * time.Millisecond)
				cancel()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
	}()
	err := within(t, 3*time.Second, "Speak", func() error { return s.Speak(ctx, "hello", Options{}) })

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Speak() error = %v, want context.Canceled", err)
	}
	if left := dirEntries(t, s.ActivityDir); len(left) != 0 {
		t.Errorf("activity dir still holds %v after cancel", left)
	}
	data, _ := os.ReadFile(afplayPID)
	if pid, _ := strconv.Atoi(strings.TrimSpace(string(data))); pid != 0 && alive(pid) {
		t.Errorf("afplay (pid %d) still running after cancel", pid)
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}

// An auto-start that "succeeds" because the caller cancelled mid-way leaves the
// engine down. That is the caller leaving, not a start that failed.
func TestCancelDuringAutoStartThatReturnsNilIsNotAFailedStart(t *testing.T) {
	engine := &stubEngine{healthy: false}
	s, rec := newTestSpeaker(t, engine, voiceconfig.Defaults())
	s.StartGuard = &startGuard{Path: filepath.Join(t.TempDir(), "failures"), Max: 1, Window: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.AutoStart = func(context.Context) error { cancel(); return nil }

	err := s.Speak(ctx, "hello", Options{})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Speak() error = %v, want context.Canceled", err)
	}
	if _, blocked := s.StartGuard.Blocked(); blocked {
		t.Error("a cancelled start was counted as a failed start")
	}
	if rec.saidText != "" {
		t.Errorf("fell back to say (%q)", rec.saidText)
	}
}

// With the lock free there is no wait to interrupt, so only the check after
// taking the lock keeps a cancelled speak from starting the player. PATH has no
// afplay at all: reaching Start would fail with a different error.
func TestPlayerNeverStartsForACancelledContext(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	s := &Speaker{LockPath: filepath.Join(t.TempDir(), "playback.lock")}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.playLocked(ctx, "clip.wav", 0, filepath.Join(t.TempDir(), "marker")+ttsproto.PlayPIDSuffix)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("playLocked() error = %v, want context.Canceled (it reached the player)", err)
	}
}

// Cancelling during the `say` fallback kills say, which exits with a signal
// error. That must come back as the cancel, not as "say fallback failed".
func TestCancelDuringSayFallbackReturnsCtxErr(t *testing.T) {
	s, rec := newTestSpeaker(t, &stubEngine{healthy: false}, settingsWith(func(c *voiceconfig.Settings) { c.EngineAutoStart = false }))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Say = func(ctx context.Context, _ string, _ int, _ string) error {
		cancel()
		return errors.New("signal: killed")
	}

	err := s.Speak(ctx, "hello", Options{})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Speak() error = %v, want context.Canceled", err)
	}
	if len(rec.played) != 0 {
		t.Errorf("played %v after cancel", rec.played)
	}
}

// The fallback synthesizer is a child process too: cancelling must not wait
// for a `say` that is mid-sentence.
func TestCancelKillsSay(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "say"), []byte("#!/bin/sh\nexec sleep 30\n"), 0755); err != nil {
		t.Fatal(err)
	}
	testutil.PrependPath(t, bin)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(100 * time.Millisecond); cancel() }()

	err := within(t, 2*time.Second, "say", func() error {
		return renderWithSay(ctx, "hello", 200, filepath.Join(t.TempDir(), "x.aiff"))
	})
	if err == nil {
		t.Error("renderWithSay() = nil after cancel, want an error")
	}
}

// Same for the engine's start script, which can run for minutes.
func TestCancelKillsTheEngineStartScript(t *testing.T) {
	script := filepath.Join(t.TempDir(), "engine.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 30\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EngineScriptEnv, script)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(100 * time.Millisecond); cancel() }()

	err := within(t, 2*time.Second, "startEngine", func() error { return startEngine(ctx) })
	if err == nil {
		t.Error("startEngine() = nil after cancel, want an error")
	}
}
