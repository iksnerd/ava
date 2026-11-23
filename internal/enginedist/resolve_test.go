package enginedist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iksnerd/ava/internal/testutil"
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
	testutil.WriteExec(t, path, "#!/bin/bash\n")
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

// `ava update` replaces the binary but not the bundle `ava setup` wrote, so a
// new ava met an older server.py and refused to speak until setup ran again.
// Resolving the installed script is the one step every start goes through, so
// that is where an out-of-date bundle is replaced.
func TestResolveScriptRefreshesAnOlderInstalledBundle(t *testing.T) {
	install := resolveEnv(t)
	touch(t, filepath.Join(install, ScriptRelPath))
	server := filepath.Join(install, "mlx-engine", "server.py")
	os.MkdirAll(filepath.Dir(server), 0755)
	os.WriteFile(server, []byte("# an older server\n"), 0644)

	if _, err := ResolveScript(""); err != nil {
		t.Fatal(err)
	}
	want, err := bundle.ReadFile("files/mlx-engine/server.py")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(server)
	if string(got) != string(want) {
		t.Error("the installed server.py is still the older one after resolving the script")
	}
}

func TestResolveScriptLeavesACurrentBundleAlone(t *testing.T) {
	install := resolveEnv(t)
	if _, err := Materialize(install); err != nil {
		t.Fatal(err)
	}
	if err := Stamp(install); err != nil {
		t.Fatal(err)
	}
	server := filepath.Join(install, "mlx-engine", "server.py")
	os.WriteFile(server, []byte("# edited on purpose\n"), 0644)

	if _, err := ResolveScript(""); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(server); string(got) != "# edited on purpose\n" {
		t.Error("a bundle that matches this binary was rewritten on every resolve")
	}
}
