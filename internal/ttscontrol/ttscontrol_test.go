package ttscontrol

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/iksnerd/ava/internal/ttsproto"
)

func TestStopSpeakingUsesEnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(ActivityDirEnv, dir)

	marker := filepath.Join(dir, "99")
	if err := os.WriteFile(marker, nil, 0644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	play := startSleeper(t)
	writePid(t, marker+".play.pid", play.Process.Pid)

	StopSpeaking()

	if _, err := os.Stat(marker + ".stopped"); err != nil {
		t.Errorf("expected %s.stopped to be created, got error: %v", marker, err)
	}
	waitExited(t, play, "play")
}

func TestStopNoActivityDir(t *testing.T) {
	stop(filepath.Join(t.TempDir(), "does-not-exist"))
}

func TestStopEmptyActivityDir(t *testing.T) {
	stop(t.TempDir())
}

func TestStopIgnoresBareSidecarFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"123.play.pid", "123.stopped"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("1"), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	stop(dir)

	// None of these are a real speak marker, so nothing should have been
	// touched or created for them.
	if _, err := os.Stat(filepath.Join(dir, "123.stopped.stopped")); err == nil {
		t.Error("stop() treated a sidecar file as a speak marker")
	}
}

// TestStopKillsTrackedProcesses simulates an in-flight speak: a marker
// file plus a .play.pid sidecar naming a real (sleeping) process, the shape
// the protocol defines. stop() should signal it and wait for the marker
// directory to empty out.
func TestStopKillsTrackedProcesses(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "4242")
	if err := os.WriteFile(marker, nil, 0644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	play := startSleeper(t)
	writePid(t, marker+".play.pid", play.Process.Pid)

	stop(dir)

	if _, err := os.Stat(marker + ".stopped"); err != nil {
		t.Errorf("expected %s.stopped to be created, got error: %v", marker, err)
	}

	waitExited(t, play, "play")
}

func startSleeper(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sleep", "5")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleeper: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return cmd
}

func writePid(t *testing.T, path string, pid int) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strconv.Itoa(pid)), 0644); err != nil {
		t.Fatalf("write pid file %s: %v", path, err)
	}
}

func waitExited(t *testing.T, cmd *exec.Cmd, label string) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Errorf("%s process was not killed", label)
	}
}

// A stop runs inside `ava` (dictation, `ava stop`, the MCP stop_speaking
// tool), and a pid file naming that same process — as the Go speaker's used
// to while queued — made it SIGTERM itself. Whatever a pid file says, a stop
// must not signal its own process.
func TestStopNeverSignalsItsOwnProcess(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "self")
	if err := os.WriteFile(marker, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker+ttsproto.PlayPIDSuffix, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		t.Fatal(err)
	}

	got := make(chan os.Signal, 1)
	signal.Notify(got, syscall.SIGTERM)
	defer signal.Stop(got)

	stop(dir)

	select {
	case <-got:
		t.Fatal("stop sent SIGTERM to its own process")
	case <-time.After(100 * time.Millisecond):
	}
}
