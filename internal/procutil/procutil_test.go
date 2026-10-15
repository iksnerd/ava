package procutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSilenceRedirectsStdoutAndStderr(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "echo out; echo err >&2")

	closer, err := Silence(cmd)
	if err != nil {
		t.Fatalf("Silence() error = %v", err)
	}
	defer closer()

	if cmd.Stdout == nil || cmd.Stderr == nil {
		t.Fatal("expected Silence to set both Stdout and Stderr")
	}
	if cmd.Stdout != cmd.Stderr {
		t.Error("expected Stdout and Stderr to be redirected to the same /dev/null handle")
	}

	if err := cmd.Run(); err != nil {
		t.Fatalf("cmd.Run() error = %v", err)
	}
}

func TestSilenceCloserReleasesTheDescriptor(t *testing.T) {
	cmd := exec.Command("/bin/true")
	closer, err := Silence(cmd)
	if err != nil {
		t.Fatalf("Silence() error = %v", err)
	}

	devNull, ok := cmd.Stdout.(*os.File)
	if !ok {
		t.Fatalf("cmd.Stdout is a %T, want *os.File", cmd.Stdout)
	}

	closer()

	if err := devNull.Close(); err == nil {
		t.Error("expected the devnull file to already be closed by the closer")
	}
}

func TestOnInterruptFiresOnSigterm(t *testing.T) {
	fired := make(chan struct{})
	OnInterrupt(func() { close(fired) })

	// signal.Notify (registered inside OnInterrupt) intercepts SIGTERM for
	// this process, so sending it to ourselves is safe here: it's delivered
	// to the channel OnInterrupt listens on instead of the default
	// terminate-the-process disposition.
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("failed to signal self: %v", err)
	}

	select {
	case <-fired:
	case <-time.After(2 * time.Second):
		t.Fatal("onSignal callback was not invoked after SIGTERM")
	}
}

func TestOnInterruptFiresOnSigint(t *testing.T) {
	fired := make(chan struct{})
	OnInterrupt(func() { close(fired) })

	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("failed to signal self: %v", err)
	}

	select {
	case <-fired:
	case <-time.After(2 * time.Second):
		t.Fatal("onSignal callback was not invoked after SIGINT")
	}
}

func processAlive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func waitGone(t *testing.T, pid int, when string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Errorf("process %d is still running %s", pid, when)
}

func readPidFile(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(path); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil {
				return pid
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no pid in %s", path)
	return 0
}

// Killing the program on a timeout is not enough when it started children: they
// keep running, and one that holds the output pipe keeps Output() waiting.
// InOwnGroup and KillGroupOnCancel put the command in a process group of its
// own and make the context's cancellation kill the group.
func TestACanceledCommandTakesItsChildrenWithIt(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 30 & echo $! > "+pidFile+"; sleep 30")
	InOwnGroup(cmd)
	KillGroupOnCancel(cmd)

	if err := cmd.Run(); err == nil {
		t.Fatal("the command finished; it should have been killed")
	}
	waitGone(t, readPidFile(t, pidFile), "after the context ended")
}

// ... and again after a normal exit, so a program that backgrounds work and
// returns leaves nothing running.
func TestReapGroupKillsWhatTheProgramLeftRunning(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	cmd := exec.Command("/bin/sh", "-c", "sleep 30 >/dev/null 2>&1 & echo $! > "+pidFile)
	InOwnGroup(cmd)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	child := readPidFile(t, pidFile)
	defer syscall.Kill(child, syscall.SIGKILL)
	if !processAlive(child) {
		t.Fatal("the fixture's child was already gone; there is nothing to reap")
	}
	ReapGroup(cmd)
	waitGone(t, child, "after ReapGroup")
}

func TestReapGroupOnACommandThatNeverStartedIsHarmless(t *testing.T) {
	ReapGroup(exec.Command("/bin/true")) // no Process yet: must not panic
}

func TestBaseEnvHasAllFourWhenAllAreSet(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", "/srv/test-home")
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("TMPDIR", "/tmp/x")
	want := "PATH=/usr/bin\nHOME=/srv/test-home\nLANG=en_US.UTF-8\nTMPDIR=/tmp/x"
	if got := strings.Join(BaseEnv(), "\n"); got != want {
		t.Errorf("BaseEnv() = %q, want %q", got, want)
	}
}

// BaseEnv is the only part of this process's environment a child program sees
// (RFC 0002 section 4): PATH, HOME, LANG and TMPDIR, the ones that are set.
func TestBaseEnvIsOnlyTheFourNamesThatAreSet(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("HOME", "/srv/test-home")
	t.Setenv("TMPDIR", "/tmp/x")
	os.Unsetenv("LANG")
	t.Setenv("API_TOKEN", "must-not-leak")
	t.Setenv("LC_ALL", "C")

	got := BaseEnv()
	want := []string{"PATH=/usr/bin:/bin", "HOME=/srv/test-home", "TMPDIR=/tmp/x"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("BaseEnv() = %q, want %q (fixed order, unset names left out, nothing else)", got, want)
	}
}

// CappedBuffer holds output up to Max. The write that would pass it keeps what
// fits, marks the buffer over, calls OnOver once and fails, so the process
// writing is stopped instead of being buffered without end.
func TestCappedBufferStopsAtItsLimit(t *testing.T) {
	calls := 0
	b := &CappedBuffer{Max: 10, OnOver: func() { calls++ }}

	if n, err := b.Write([]byte("12345")); n != 5 || err != nil {
		t.Fatalf("under the cap: n=%d err=%v", n, err)
	}
	if n, err := b.Write([]byte("67890")); n != 5 || err != nil || b.Over() {
		t.Fatalf("exactly at the cap must be allowed: n=%d err=%v over=%v", n, err, b.Over())
	}
	if _, err := b.Write([]byte("x")); err == nil || !b.Over() {
		t.Fatalf("one byte past the cap must fail: err=%v over=%v", err, b.Over())
	}
	if _, err := b.Write([]byte("more")); err == nil {
		t.Error("a write after the cap succeeded")
	}
	if string(b.Bytes()) != "1234567890" {
		t.Errorf("kept %q, want the first 10 bytes only", b.Bytes())
	}
	if calls != 1 {
		t.Errorf("OnOver ran %d times, want once", calls)
	}
}

func TestCappedBufferKeepsWhatFitsOfAWriteThatCrossesTheCap(t *testing.T) {
	b := &CappedBuffer{Max: 4}
	if n, err := b.Write([]byte("abcdefgh")); err == nil || n != 4 || string(b.Bytes()) != "abcd" {
		t.Errorf("n=%d err=%v kept=%q, want 4 bytes kept and an error", n, err, b.Bytes())
	}
}

// HeadBuffer keeps the first Max bytes and reads the rest without failing, so a
// chatty program neither fills memory nor blocks on a full pipe.
func TestHeadBufferKeepsTheStartAndDiscardsTheRest(t *testing.T) {
	b := &HeadBuffer{Max: 6}
	for _, chunk := range []string{"abc", "defg", "hij"} {
		if n, err := b.Write([]byte(chunk)); n != len(chunk) || err != nil {
			t.Fatalf("Write(%q) = %d, %v; a head buffer never fails or short-writes", chunk, n, err)
		}
	}
	if string(b.Bytes()) != "abcdef" {
		t.Errorf("kept %q, want the first 6 bytes", b.Bytes())
	}
}
