package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iksnerd/local-whisper/internal/testutil"
)

func TestCheckDependencies(t *testing.T) {
	t.Run("sox missing", func(t *testing.T) {
		testutil.SetPath(t, t.TempDir())
		err := checkDependencies()
		if err == nil || !strings.Contains(err.Error(), "sox is not installed") {
			t.Errorf("err = %v, want it to mention sox is not installed", err)
		}
	})

	t.Run("whisper-cli missing", func(t *testing.T) {
		soxOnly := t.TempDir()
		copyFixtureBin(t, soxOnly, "sox")
		testutil.SetPath(t, soxOnly)

		err := checkDependencies()
		if err == nil || !strings.Contains(err.Error(), "whisper-cli is not installed") {
			t.Errorf("err = %v, want it to mention whisper-cli is not installed", err)
		}
	})

	t.Run("everything present", func(t *testing.T) {
		testutil.SetPath(t, "testdata/bin")
		if err := checkDependencies(); err != nil {
			t.Errorf("checkDependencies() = %v, want nil", err)
		}
	})

}

// copyFixtureBin copies a fixture executable from cmd/local-whisper/testdata/bin
// into dir, so a test can compose a PATH with only a subset of dependencies present.
func copyFixtureBin(t *testing.T, dir, name string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", "bin", name))
	if err != nil {
		t.Fatalf("read fixture bin %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), content, 0755); err != nil {
		t.Fatalf("write fixture bin %s: %v", name, err)
	}
}
