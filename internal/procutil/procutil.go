// Package procutil holds small subprocess helpers shared across the CLIs
// and engine clients.
package procutil

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// Silence redirects cmd's stdout and stderr to /dev/null. Call the returned
// closer once cmd has finished running to release the descriptor.
func Silence(cmd *exec.Cmd) (func(), error) {
	// Opened write-only: cmd writes into this fd, and a program that (unlike
	// most) checks fprintf's return value will get EBADF from a read-only
	// handle and can treat that as a fatal error.
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return nil, err
	}
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	return func() { devNull.Close() }, nil
}

// OnInterrupt spawns a goroutine that waits for SIGINT or SIGTERM and then
// calls onSignal. It returns immediately; typical use is to register cleanup
// before starting a blocking subprocess or server that the same signal will
// also terminate.
func OnInterrupt(onSignal func()) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		onSignal()
	}()
}

// InOwnGroup puts cmd in a process group of its own, so the whole group can be
// signalled: its children are in it too. Call it before Start. Killing only the
// program is not enough when it starts children: they keep running after the
// error is reported, and one that holds the output pipe keeps Output() waiting.
func InOwnGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// KillGroupOnCancel makes the end of the context a command was made with
// (exec.CommandContext's timeout or cancel) kill its whole process group, not
// only the program. It needs InOwnGroup, and must be called on a command made
// by CommandContext, which is the only kind Go lets have a Cancel.
func KillGroupOnCancel(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			if errors.Is(err, syscall.ESRCH) {
				return os.ErrProcessDone // the group is already gone
			}
			return err
		}
		return nil
	}
}

// ReapGroup kills anything still running in cmd's process group once cmd has
// finished, so a program that backgrounds work and returns leaves nothing
// behind. A group with no members left, the usual case, is not an error. Safe
// to call on a command that never started.
func ReapGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
