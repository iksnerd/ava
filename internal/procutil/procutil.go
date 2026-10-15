// Package procutil holds small subprocess helpers shared across the CLIs
// and engine clients.
package procutil

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"sync"
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

// baseEnv are the only names of this process's environment a child program is
// given (RFC 0002 section 4): where to find programs, the home directory, the
// locale and the temp directory.
var baseEnv = []string{"PATH", "HOME", "LANG", "TMPDIR"}

// BaseEnv returns that part of the environment, as NAME=value, in a fixed order
// and leaving out the names that are not set. Assign it to exec.Cmd.Env: a
// program that gets the whole environment gets every token and key the caller
// happens to hold, which it has no business with.
func BaseEnv() []string {
	var env []string
	for _, name := range baseEnv {
		if v, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+v)
		}
	}
	return env
}

// ErrOverCap is what CappedBuffer.Write returns once the cap is passed.
var ErrOverCap = errors.New("output over the cap")

// CappedBuffer holds a program's output up to Max bytes. The write that would
// pass it keeps what fits, marks the buffer over, calls OnOver (once) and fails,
// so the program writing can be stopped instead of buffered without end. Safe
// for the concurrent use exec makes of it.
type CappedBuffer struct {
	Max    int
	OnOver func()

	mu   sync.Mutex
	buf  []byte
	over bool
}

func (b *CappedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	if room := b.Max - len(b.buf); len(p) > room {
		b.buf = append(b.buf, p[:room]...)
		first := !b.over
		b.over = true
		b.mu.Unlock()
		if first && b.OnOver != nil {
			b.OnOver()
		}
		return room, ErrOverCap
	}
	b.buf = append(b.buf, p...)
	b.mu.Unlock()
	return len(p), nil
}

// Over reports whether more than Max bytes were written.
func (b *CappedBuffer) Over() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.over
}

// Bytes is what was kept: at most Max bytes.
func (b *CappedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf
}

// HeadBuffer keeps the first Max bytes and reads and discards the rest without
// failing, so a chatty program (stderr) neither fills memory nor blocks on a
// full pipe.
type HeadBuffer struct {
	Max int

	mu  sync.Mutex
	buf []byte
}

func (b *HeadBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if room := b.Max - len(b.buf); room > 0 {
		b.buf = append(b.buf, p[:min(room, len(p))]...)
	}
	return len(p), nil
}

// Bytes is what was kept: at most Max bytes.
func (b *HeadBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf
}
