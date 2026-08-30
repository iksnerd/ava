// Package testutil holds small helpers shared by this repo's test suites.
// It is a regular (non-_test.go) package so it can be imported from the
// _test.go files of any other package.
package testutil

import (
	"os"
	"path/filepath"
	"strings"
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
