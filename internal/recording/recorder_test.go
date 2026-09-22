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
	synth := exec.Command("sleep", "5")
	if err := synth.Start(); err != nil {
		t.Fatalf("start fake in-flight synth process: %v", err)
	}
	t.Cleanup(func() { _ = synth.Process.Kill() })
	if err := os.WriteFile(marker+".synth.pid", []byte(strconv.Itoa(synth.Process.Pid)), 0644); err != nil {
		t.Fatalf("write synth pid file: %v", err)
	}

	r := NewRecorder(filepath.Join(t.TempDir(), "audio.wav"), false)
	if err := r.Record(); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	if _, err := os.Stat(marker + ".stopped"); err != nil {
		t.Errorf("expected the in-flight speak's %s.stopped marker to exist, got error: %v", marker, err)
	}

	done := make(chan error, 1)
	go func() { done <- synth.Wait() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("Record() did not stop the in-flight synth process")
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
