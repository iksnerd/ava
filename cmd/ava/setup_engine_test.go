package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/iksnerd/ava/internal/enginedist"
	"github.com/iksnerd/ava/internal/testutil"
)

// shortEngineDir is an install dir under /tmp: t.TempDir() is long enough on
// macOS to break espeak's 160-byte path budget, which setup refuses up front.
func shortEngineDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ava-e")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv(enginedist.DirEnv, dir)
	t.Setenv(enginedist.LegacyDirEnv, "")
	return dir
}

// stubUv records where and how it ran, and creates the venv's uvicorn the
// way a real `uv sync` would, so the result looks like a usable engine.
func stubUv(t *testing.T) (record string) {
	t.Helper()
	bin := t.TempDir()
	record = filepath.Join(t.TempDir(), "uv-ran")
	script := "#!/bin/sh\necho \"$PWD $*\" > " + record + "\nmkdir -p .venv/bin && touch .venv/bin/uvicorn\n"
	if err := os.WriteFile(filepath.Join(bin, "uv"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	testutil.PrependPath(t, bin)
	return record
}

func TestSetupEngineInstallsARunnableBundle(t *testing.T) {
	if runtime.GOARCH != "arm64" {
		t.Skip("setupEngine skips the engine off Apple Silicon")
	}
	dir := shortEngineDir(t)
	ran := stubUv(t)

	if err := setupEngine(&bytes.Buffer{}); err != nil {
		t.Fatalf("setupEngine: %v", err)
	}
	got, _ := os.ReadFile(ran)
	if want := filepath.Join(dir, "mlx-engine") + " sync"; !strings.HasPrefix(string(got), want) {
		t.Errorf("uv ran as %q, want `uv sync` in %s/mlx-engine", got, dir)
	}
	// What speech auto-start and `ava engine` will then find.
	script, ok := enginedist.InstalledScript()
	if !ok || script != filepath.Join(dir, enginedist.ScriptRelPath) {
		t.Errorf("InstalledScript() = %q, %v after setup", script, ok)
	}
	if info, err := os.Stat(script); err != nil || info.Mode()&0111 == 0 {
		t.Errorf("control script %s is not executable: %v", script, err)
	}
}

func TestSetupEngineWithoutUvSaysSoAndKeepsTheBundle(t *testing.T) {
	if runtime.GOARCH != "arm64" {
		t.Skip("setupEngine skips the engine off Apple Silicon")
	}
	dir := shortEngineDir(t)
	testutil.SetPath(t, t.TempDir()) // no uv anywhere

	err := setupEngine(&bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "uv") {
		t.Fatalf("setupEngine without uv: err = %v, want it to name uv", err)
	}
	// The re-run only has the uv step left, which the message promises.
	if _, ok := enginedist.InstalledScript(); !ok {
		t.Errorf("the bundle was not left unpacked in %s for the re-run", dir)
	}
}

func TestSetupEngineRefusesATooLongPathBeforeWritingAnything(t *testing.T) {
	if runtime.GOARCH != "arm64" {
		t.Skip("setupEngine skips the engine off Apple Silicon")
	}
	long := filepath.Join(t.TempDir(), strings.Repeat("x", 120))
	t.Setenv(enginedist.DirEnv, long)
	stubUv(t)

	if err := setupEngine(&bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("setupEngine at a %d-char path: err = %v, want a refusal", len(long), err)
	}
	if _, err := os.Stat(long); !os.IsNotExist(err) {
		t.Error("setup wrote into the too-long path before refusing it")
	}
}
