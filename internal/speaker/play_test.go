package speaker

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/iksnerd/ava/internal/testutil"
	"github.com/iksnerd/ava/internal/ttsproto"
)

// stubAfplay puts an afplay on PATH that records its own PID and plays for
// a moment, so playLocked runs for real without making a sound.
func stubAfplay(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	pidOut := filepath.Join(t.TempDir(), "afplay.pid")
	script := "#!/bin/sh\necho $$ > " + pidOut + "\nsleep 0.2\n"
	if err := os.WriteFile(filepath.Join(bin, "afplay"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	testutil.PrependPath(t, bin)
	return pidOut
}

// holdLock takes the playback lock the way another speaker would, so the
// speaker under test has to queue.
func holdLock(t *testing.T, path string) (release func()) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }
}

// A speak waiting its turn used to write its own PID into .play.pid, and
// every stop SIGTERMs that file's PID: the menu bar's Stop, `ava stop`, or a
// dictation starting killed the whole process, which for `ava mcp` is the
// MCP server. The PID file may only ever name the player.
func TestQueuedSpeakNeverNamesItsOwnProcess(t *testing.T) {
	stubAfplay(t)
	s := &Speaker{LockPath: filepath.Join(t.TempDir(), "playback.lock")}
	marker := filepath.Join(t.TempDir(), "marker")
	pidFile := marker + ttsproto.PlayPIDSuffix

	release := holdLock(t, s.LockPath)
	done := make(chan error, 1)
	go func() { done <- s.playLocked("clip.wav", 0, pidFile) }()

	for i := 0; i < 10; i++ {
		time.Sleep(20 * time.Millisecond)
		if data, err := os.ReadFile(pidFile); err == nil &&
			strings.TrimSpace(string(data)) == strconv.Itoa(os.Getpid()) {
			release()
			<-done
			t.Fatal("a queued speak wrote its own PID to the play pid file, so any stop would kill this process")
		}
	}
	release()
	if err := <-done; err != nil {
		t.Fatalf("playLocked after the lock was released: %v", err)
	}
}

// Without a PID to signal, a queued speak must still be cancellable: a stop
// writes the .stopped sidecar first, and the waiter has to notice it and give
// up rather than play once the lock frees.
func TestQueuedSpeakGivesUpWhenStopped(t *testing.T) {
	afplayPID := stubAfplay(t)
	s := &Speaker{LockPath: filepath.Join(t.TempDir(), "playback.lock")}
	marker := filepath.Join(t.TempDir(), "marker")

	release := holdLock(t, s.LockPath)
	defer release()
	done := make(chan error, 1)
	go func() { done <- s.playLocked("clip.wav", 0, marker+ttsproto.PlayPIDSuffix) }()

	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(marker+ttsproto.StoppedSuffix, nil, 0644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("a stopped speak kept waiting for the playback lock")
	}
	if _, err := os.Stat(afplayPID); err == nil {
		t.Error("a speak stopped while queued still started afplay")
	}
}

// Once it has the lock, the PID file names the player, which is what a stop
// has to kill to silence it.
func TestPlayingSpeakNamesThePlayer(t *testing.T) {
	afplayPID := stubAfplay(t)
	s := &Speaker{LockPath: filepath.Join(t.TempDir(), "playback.lock")}
	pidFile := filepath.Join(t.TempDir(), "marker") + ttsproto.PlayPIDSuffix

	done := make(chan error, 1)
	go func() { done <- s.playLocked("clip.wav", 0, pidFile) }()

	var named string
	for i := 0; i < 50 && named == ""; i++ {
		time.Sleep(10 * time.Millisecond)
		if data, err := os.ReadFile(pidFile); err == nil {
			named = strings.TrimSpace(string(data))
		}
	}
	if err := <-done; err != nil {
		t.Fatalf("playLocked: %v", err)
	}
	player, _ := os.ReadFile(afplayPID)
	if named == "" || named != strings.TrimSpace(string(player)) {
		t.Errorf("play pid file named %q during playback, want afplay's PID %q", named, strings.TrimSpace(string(player)))
	}
	if _, err := os.Stat(pidFile); err == nil {
		t.Error("play pid file left behind after playback")
	}
}
