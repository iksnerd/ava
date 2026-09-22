package enginedist

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The bundle's file list is a hand-maintained answer to "what does the engine
// need to run", and it drifted from the truth within hours of being written:
// server.py gained `import protocol` the same day, and the first real install
// died on ModuleNotFoundError after uv had already resolved 1.2 GB.
//
// This is the cheap version of catching that — every local import in the
// bundled Python must resolve to a module the bundle also carries. It does not
// prove the server runs; it does prove the file list is not missing a piece,
// which is the failure that actually happened.
func TestBundledPythonHasEveryLocalImport(t *testing.T) {
	dir := t.TempDir()
	if _, err := Materialize(dir); err != nil {
		t.Fatalf("materialize: %v", err)
	}

	engineDir := filepath.Join(dir, "mlx-engine")
	entries, err := os.ReadDir(engineDir)
	if err != nil {
		t.Fatalf("read bundled mlx-engine: %v", err)
	}
	present := map[string]bool{}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".py") {
			present[strings.TrimSuffix(e.Name(), ".py")] = true
		}
	}
	if len(present) == 0 {
		t.Fatal("the bundle carries no Python at all — the file list lost mlx-engine/")
	}

	// Third-party imports come from uv.lock; only a bare `import x` for an x
	// that exists as a sibling .py in the repo is our problem.
	repoEngine := filepath.Join("..", "..", "mlx-engine")
	repoEntries, err := os.ReadDir(repoEngine)
	if err != nil {
		t.Fatalf("read repo mlx-engine: %v", err)
	}
	local := map[string]bool{}
	for _, e := range repoEntries {
		if strings.HasSuffix(e.Name(), ".py") {
			local[strings.TrimSuffix(e.Name(), ".py")] = true
		}
	}

	importRe := regexp.MustCompile(`(?m)^\s*(?:import|from)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	for name := range present {
		body, err := os.ReadFile(filepath.Join(engineDir, name+".py"))
		if err != nil {
			t.Fatalf("read bundled %s.py: %v", name, err)
		}
		for _, m := range importRe.FindAllStringSubmatch(string(body), -1) {
			mod := m[1]
			if !local[mod] || present[mod] {
				continue
			}
			t.Errorf("bundled %s.py does `import %s`, and %s.py exists in the repo "+
				"but is not in the bundle — a real install dies on ModuleNotFoundError "+
				"after uv has already resolved a gigabyte. Add it to `sources` in "+
				"internal/enginedist/gen.", name, mod, mod)
		}
	}
}

// The control script has to be executable when it lands: embed.FS drops the
// mode bit, and a bundle whose script is 0644 fails at `engine start`, nowhere
// near the code that wrote it.
func TestMaterializedControlScriptIsExecutable(t *testing.T) {
	dir := t.TempDir()
	if _, err := Materialize(dir); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, ScriptRelPath))
	if err != nil {
		t.Fatalf("stat control script: %v", err)
	}
	if info.Mode().Perm()&0111 == 0 {
		t.Errorf("%s materialized as %v, not executable", ScriptRelPath, info.Mode().Perm())
	}
}

// Materialize overwrites so that a new binary replaces an older bundle.
func TestMaterializeOverwritesAnOlderBundle(t *testing.T) {
	dir := t.TempDir()
	if _, err := Materialize(dir); err != nil {
		t.Fatalf("first materialize: %v", err)
	}
	script := filepath.Join(dir, ScriptRelPath)
	if err := os.WriteFile(script, []byte("#!/bin/bash\n# stale\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Materialize(dir); err != nil {
		t.Fatalf("second materialize: %v", err)
	}
	body, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "# stale") {
		t.Error("an existing bundle was left in place; a binary upgrade would keep " +
			"running the old server against a new client")
	}
	if info, _ := os.Stat(script); info != nil && info.Mode().Perm()&0111 == 0 {
		t.Error("overwriting an existing file left it non-executable")
	}
}

// go:embed drops dot-prefixed entries unless the pattern says all:, and the
// one that matters here pins the Python version. Without it uv picks a
// version of its own, and a fresh install dies inside espeak's data files
// after resolving a gigabyte — far from anything that names Python.
func TestBundleCarriesThePythonPin(t *testing.T) {
	dir := t.TempDir()
	if _, err := Materialize(dir); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	pin := filepath.Join(dir, "mlx-engine", ".python-version")
	body, err := os.ReadFile(pin)
	if err != nil {
		t.Fatalf("the bundle has no mlx-engine/.python-version (%v). "+
			"Check that the embed pattern is `all:files` — a plain `files` "+
			"skips dot-prefixed entries without saying so.", err)
	}
	if strings.TrimSpace(string(body)) == "" {
		t.Error("mlx-engine/.python-version materialized empty")
	}
}
