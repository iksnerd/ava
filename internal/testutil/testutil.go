// Package testutil holds small helpers shared by this repo's test suites.
// It is a regular (non-_test.go) package so it can be imported from the
// _test.go files of any other package.
package testutil

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// PrependPath prepends dir to PATH for the duration of the test (restored
// automatically by t.Setenv's cleanup). Use it to point a bare command name
// (as exec.Command/exec.LookPath resolve it) at a fixture executable under
// testdata/bin instead of the real system binary.
func PrependPath(t testing.TB, dir string) {
	t.Helper()
	t.Setenv("PATH", absPath(t, dir)+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// SetPath replaces PATH outright for the duration of the test (restored
// automatically by t.Setenv's cleanup), with no fallback to the real system
// PATH. Use it, unlike PrependPath, when a test needs a command to be
// genuinely absent rather than just shadowed.
func SetPath(t testing.TB, dirs ...string) {
	t.Helper()
	abs := make([]string, len(dirs))
	for i, dir := range dirs {
		abs[i] = absPath(t, dir)
	}
	t.Setenv("PATH", strings.Join(abs, string(os.PathListSeparator)))
}

func absPath(t testing.TB, dir string) string {
	t.Helper()
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("testutil: filepath.Abs(%q): %v", dir, err)
	}
	return abs
}

// WriteExec writes body to path with mode 0755 (a stub executable), failing
// the test on error.
func WriteExec(t testing.TB, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0755); err != nil {
		t.Fatalf("testutil: write %s: %v", path, err)
	}
}

// FreePort returns a TCP port on 127.0.0.1 that was free a moment ago.
func FreePort(t testing.TB) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("testutil: listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// RepoPath returns the absolute path of parts under the repo root. It assumes
// the calling test's package directory is exactly two levels below the root
// (cmd/ava, internal/x, pkg/x).
func RepoPath(t testing.TB, parts ...string) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join(append([]string{"..", ".."}, parts...)...))
	if err != nil {
		t.Fatalf("testutil: filepath.Abs: %v", err)
	}
	return abs
}

// RepoFile reads a file under the repo root; see RepoPath.
func RepoFile(t testing.TB, parts ...string) []byte {
	t.Helper()
	data, err := os.ReadFile(RepoPath(t, parts...))
	if err != nil {
		t.Fatalf("testutil: %v", err)
	}
	return data
}

// Alive reports whether a process with this PID exists.
func Alive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// DeadPID returns the PID of a process that has exited and been reaped, so
// kill(pid, 0) reports ESRCH.
func DeadPID(t testing.TB) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("testutil: run true: %v", err)
	}
	return cmd.Process.Pid
}
