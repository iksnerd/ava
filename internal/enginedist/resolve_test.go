package enginedist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ResolveScript is the one answer to "which control script runs the
// engine", for `ava engine` and for speech auto-start. They used to resolve
// it separately and disagreed: `ava engine` ignored MLX_ENGINE_SCRIPT, so with
// it set, auto-start and `ava engine stop` drove different scripts.

// resolveEnv gives each test a clean slate: no override, no checkout in the
// working directory, and an installed bundle only where the test puts one.
func resolveEnv(t *testing.T) (installDir string) {
	t.Helper()
	t.Setenv(ScriptEnv, "")
	installDir = t.TempDir()
	t.Setenv(DirEnv, installDir)
	t.Chdir(t.TempDir())
	return installDir
}

func touch(t *testing.T, path string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte("#!/bin/bash\n"), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestResolveScriptPrefersTheFlag(t *testing.T) {
	resolveEnv(t)
	t.Setenv(ScriptEnv, "/from/env.sh")
	if got, err := ResolveScript("/from/flag.sh"); err != nil || got != "/from/flag.sh" {
		t.Errorf("ResolveScript(flag) = %q, %v; want the flag", got, err)
	}
}

func TestResolveScriptHonoursTheEnvironment(t *testing.T) {
	install := resolveEnv(t)
	touch(t, filepath.Join(install, ScriptRelPath))
	touch(t, ScriptRelPath) // a checkout in the working directory
	t.Setenv(ScriptEnv, "/from/env.sh")
	if got, err := ResolveScript(""); err != nil || got != "/from/env.sh" {
		t.Errorf("ResolveScript() = %q, %v; want %s to win", got, err, ScriptEnv)
	}
}

func TestResolveScriptUsesACheckoutInTheWorkingDirectory(t *testing.T) {
	install := resolveEnv(t)
	touch(t, filepath.Join(install, ScriptRelPath))
	touch(t, ScriptRelPath)
	if got, err := ResolveScript(""); err != nil || got != ScriptRelPath {
		t.Errorf("ResolveScript() = %q, %v; want the checkout's %s", got, err, ScriptRelPath)
	}
}

func TestResolveScriptFallsBackToTheInstalledBundle(t *testing.T) {
	install := resolveEnv(t)
	want := filepath.Join(install, ScriptRelPath)
	touch(t, want)
	if got, err := ResolveScript(""); err != nil || got != want {
		t.Errorf("ResolveScript() = %q, %v; want the installed %s", got, err, want)
	}
}

func TestResolveScriptSaysWhereItLooked(t *testing.T) {
	resolveEnv(t)
	_, err := ResolveScript("")
	if err == nil {
		t.Fatal("ResolveScript found a script where there is none")
	}
	for _, want := range []string{"ava setup", ScriptEnv, ScriptRelPath} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}
