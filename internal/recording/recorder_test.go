package recording

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/iksnerd/ava/internal/testutil"
	"github.com/iksnerd/ava/internal/ttscontrol"
)

// isolateTTSActivityDir points Record()'s ttscontrol.StopSpeaking() call at
// an empty, private directory instead of the real, shared
// /tmp/ava-tts-active — otherwise a test run could kill actual
// in-flight speech on the machine running the tests. Returns the directory
// so callers that want to plant a fake in-flight speak can do so.
func isolateTTSActivityDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "tts-active")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("create isolated tts activity dir: %v", err)
	}
	t.Setenv(ttscontrol.ActivityDirEnv, dir)
	return dir
}

func TestNewRecorder(t *testing.T) {
	outputPath := "/tmp/audio.wav"
	playSound := true

	recorder := NewRecorder(outputPath, playSound)

	if recorder.OutputPath != outputPath {
		t.Errorf("expected OutputPath %q, got %q", outputPath, recorder.OutputPath)
	}

	if recorder.PlaySound != playSound {
		t.Errorf("expected PlaySound %v, got %v", playSound, recorder.PlaySound)
	}
}

func TestRecordSuccess(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin")
	isolateTTSActivityDir(t)

	dir := t.TempDir()
	outputPath := filepath.Join(dir, "audio.wav")
	logPath := filepath.Join(dir, "sox.log")
	t.Setenv("SOX_LOG", logPath)

	r := NewRecorder(outputPath, false)
	if err := r.Record(); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if want := "recorded audio\n"; string(got) != want {
		t.Errorf("output content = %q, want %q", got, want)
	}

	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read sox invocation log: %v", err)
	}
	for _, want := range []string{"-d", "-r", "16000", "-c", "1", "silence", "norm", "-3"} {
		if !strings.Contains(string(log), want) {
			t.Errorf("sox args = %q, want it to contain %q", log, want)
		}
	}
}

func TestRecordPlaysStartSoundWhenEnabled(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin")
	isolateTTSActivityDir(t)

	dir := t.TempDir()
	afplayLog := filepath.Join(dir, "afplay.log")
	t.Setenv("AFPLAY_LOG", afplayLog)

	r := NewRecorder(filepath.Join(dir, "audio.wav"), true)
	if err := r.Record(); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	// Record() waits for the start sound to finish before sox starts
	// listening (so the beep itself can't bleed into the recording), so its
	// side effect is already visible by the time Record() returns.
	content, err := os.ReadFile(afplayLog)
	if err != nil {
		t.Fatalf("read afplay invocation log: %v", err)
	}
	if got := strings.TrimSpace(string(content)); got != "/System/Library/Sounds/Blow.aiff" {
		t.Errorf("afplay was called with %q, want the Blow.aiff start sound", got)
	}
}

func TestRecordSkipsStartSoundWhenDisabled(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin")
	isolateTTSActivityDir(t)

	dir := t.TempDir()
	afplayLog := filepath.Join(dir, "afplay.log")
	t.Setenv("AFPLAY_LOG", afplayLog)

	r := NewRecorder(filepath.Join(dir, "audio.wav"), false)
	if err := r.Record(); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	if _, err := os.Stat(afplayLog); err == nil {
		t.Error("expected afplay not to be invoked when PlaySound is false")
	}
}

func TestRecordStopsInFlightTTSBeforeRecording(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin")
	activityDir := isolateTTSActivityDir(t)

	marker := filepath.Join(activityDir, "123")
	if err := os.WriteFile(marker, nil, 0644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	player := exec.Command("sleep", "5")
	if err := player.Start(); err != nil {
		t.Fatalf("start fake in-flight player: %v", err)
	}
	t.Cleanup(func() { _ = player.Process.Kill() })
	if err := os.WriteFile(marker+".play.pid", []byte(strconv.Itoa(player.Process.Pid)), 0644); err != nil {
		t.Fatalf("write play pid file: %v", err)
	}

	r := NewRecorder(filepath.Join(t.TempDir(), "audio.wav"), false)
	if err := r.Record(); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	if _, err := os.Stat(marker + ".stopped"); err != nil {
		t.Errorf("expected the in-flight speak's %s.stopped marker to exist, got error: %v", marker, err)
	}

	done := make(chan error, 1)
	go func() { done <- player.Wait() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("Record() did not stop the in-flight player process")
	}
}

func TestRecordCommandFailure(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin-fail")
	isolateTTSActivityDir(t)

	dir := t.TempDir()
	r := NewRecorder(filepath.Join(dir, "audio.wav"), false)

	if err := r.Record(); err == nil {
		t.Fatal("expected an error when sox fails, got nil")
	}
}

// bin-trap's sox keeps "recording" until SIGINT, then writes the file and
// exits 130, as real sox does when interrupted.
func TestStopFinalizesAndKeepsAudio(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin-trap")
	isolateTTSActivityDir(t)

	dir := t.TempDir()
	out := filepath.Join(dir, "audio.wav")
	t.Setenv("SOX_READY", filepath.Join(dir, "ready"))
	r := NewRecorder(out, false)

	done := make(chan error, 1)
	go func() { done <- r.Record() }()
	waitForFile(t, filepath.Join(dir, "ready"))

	r.Stop()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Record() after Stop = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Record() did not return after Stop")
	}
	if b, _ := os.ReadFile(out); string(b) != "finalized\n" {
		t.Errorf("output = %q, want the finalized audio", b)
	}
}

func TestKillEndsSoxWithoutWaitingForIt(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin-trap")
	isolateTTSActivityDir(t)

	dir := t.TempDir()
	t.Setenv("SOX_READY", filepath.Join(dir, "ready"))
	r := NewRecorder(filepath.Join(dir, "audio.wav"), false)

	done := make(chan error, 1)
	go func() { done <- r.Record() }()
	waitForFile(t, filepath.Join(dir, "ready"))

	r.Kill()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Record() did not return after Kill")
	}
	if _, err := os.Stat(filepath.Join(dir, "audio.wav")); err == nil {
		t.Error("a killed sox must not have written the file")
	}
}

func TestStopBeforeStartSkipsSox(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin")
	isolateTTSActivityDir(t)

	dir := t.TempDir()
	out := filepath.Join(dir, "audio.wav")
	r := NewRecorder(out, false)
	r.Stop()
	if err := r.Record(); err != ErrStoppedBeforeStart {
		t.Fatalf("Record() = %v, want ErrStoppedBeforeStart", err)
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("sox ran although recording was stopped before it started")
	}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s never appeared", path)
}
