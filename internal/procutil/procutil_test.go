package procutil

import (
	"os"
	"os/exec"
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
