// Command gen copies the engine bundle's canonical files into
// internal/enginedist/files so they can be embedded. Run it with
// `make generate-enginedist`; `make check-enginedist` fails when a copy is
// stale.
//
// The copies exist only because go:embed cannot reach outside its own
// package — the same constraint that made internal/protocol a generator
// rather than a file everyone reads. Treating them as generated, checked
// output is what keeps them from becoming the hand-written second copies this
// repo keeps having to hunt down.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// The bundle deliberately mirrors the repo's own layout — scripts/ with
// mlx-engine/ as its sibling — because mlx-engine-server.sh derives ROOT_DIR
// from its own location and then uses "$ROOT_DIR/mlx-engine". Keeping the
// shape means the script runs unmodified once materialized.

// scriptFiles are the only parts of scripts/ the engine needs. This one is a
// list because scripts/ holds plenty the engine has nothing to do with.
var scriptFiles = []string{
	"mlx-engine-server.sh",
	"lib.sh",
	"protocol.sh",
	"voice-defaults.json",
}

// engineSkip names what must NOT travel from mlx-engine/: build output,
// caches, and the tests and docs a running server has no use for.
var engineSkip = map[string]bool{
	".venv": true, "__pycache__": true, ".pytest_cache": true,
	".ruff_cache": true, "tests": true, "README.md": true, "Makefile": true,
}

// engineFiles enumerates mlx-engine/ rather than listing it, because a
// hand-written list of "what the engine needs" is a copy of the truth, and
// this one drifted twice in the hour it existed: server.py's `import protocol`
// died after uv had already resolved a gigabyte, and a missing .python-version
// let uv pick a Python the espeak data files do not work on. Enumerating means
// adding a file to mlx-engine/ is enough.
func engineFiles(root string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, "mlx-engine"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if engineSkip[e.Name()] || e.IsDir() {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out, nil
}

// buildSources maps a path inside the bundle to its canonical path in the repo.
func buildSources(root string) (map[string]string, error) {
	sources := map[string]string{}
	for _, f := range scriptFiles {
		sources["scripts/"+f] = "scripts/" + f
	}
	engine, err := engineFiles(root)
	if err != nil {
		return nil, err
	}
	for _, f := range engine {
		sources["mlx-engine/"+f] = "mlx-engine/" + f
	}
	return sources, nil
}

func main() {
	root, err := repoRoot()
	if err != nil {
		fail(err)
	}
	dest := filepath.Join(root, "internal", "enginedist", "files")
	check := len(os.Args) > 1 && os.Args[1] == "-check"

	sources, err := buildSources(root)
	if err != nil {
		fail(err)
	}

	var stale []string
	for rel, src := range sources {
		want, err := os.ReadFile(filepath.Join(root, src))
		if err != nil {
			fail(fmt.Errorf("read canonical %s: %w", src, err))
		}
		out := filepath.Join(dest, rel)
		if check {
			got, err := os.ReadFile(out)
			if err != nil || string(got) != string(want) {
				stale = append(stale, rel)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
			fail(err)
		}
		// Keep the executable bit on scripts: a materialized bundle whose
		// control script is not executable fails at the point of use, far
		// from here.
		mode := os.FileMode(0644)
		if filepath.Ext(rel) == ".sh" {
			mode = 0755
		}
		if err := os.WriteFile(out, want, mode); err != nil {
			fail(err)
		}
		fmt.Println("wrote internal/enginedist/files/" + rel)
	}

	if check {
		if len(stale) > 0 {
			fmt.Fprintln(os.Stderr, "❌ Engine bundle copies are stale:")
			for _, s := range stale {
				fmt.Fprintln(os.Stderr, "  "+s)
			}
			fmt.Fprintln(os.Stderr, "\n   Run `make generate-enginedist` and commit the result.")
			os.Exit(1)
		}
		fmt.Printf("✅ Engine bundle is current (%d files)\n", len(sources))
	}
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod above the working directory")
		}
		dir = parent
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "enginedist gen:", err)
	os.Exit(1)
}
