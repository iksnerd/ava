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
