// Package enginedist carries the mlx-engine bundle inside the binary so that
// Kokoro TTS can be installed without a checkout.
//
// The problem it solves: ava is usable from a single binary for
// everything except speech, and speech is the feature the project leads with.
// `engine start` shells out to scripts/mlx-engine-server.sh, which drives the
// Python project in mlx-engine/ — neither of which exists on a machine that
// only has the binary, so `speak` silently degrades to the macOS `say` voice.
//
// What travels is small: about 600 KB of scripts, server.py and a uv.lock.
// The 1.2 GB venv is resolved by `uv sync` at setup time and the 339 MB
// Kokoro model is fetched by the server on first use, so neither is shipped.
//
// The bundle mirrors the repo's own layout, with scripts/ and mlx-engine/ as
// siblings, because mlx-engine-server.sh derives its root from its own
// location. Keeping the shape means the materialized script is byte-identical
// to the one in the repo rather than a variant that has to be maintained.
package enginedist

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// all: is load-bearing. A plain `//go:embed files` silently skips entries
// beginning with "." or "_", which would drop mlx-engine/.python-version —
// and without that pin uv resolves whatever Python it likes, which is how a
// fresh install landed on 3.14 and died in espeak's data files after
// resolving 1.2 GB.
//
//go:embed all:files
var bundle embed.FS

// ScriptRelPath is where the control script lands inside a materialized
// bundle, relative to its root.
const ScriptRelPath = "scripts/mlx-engine-server.sh"

// DirEnv overrides DefaultDir. It exists so the install can be exercised
// against a throwaway directory instead of the real one — the same escape
// hatch VOICECONFIG_PATH and TTSCONTROL_ACTIVITY_DIR provide for the config
// and the marker directory, and for the same reason: a test that can only run
// against live state does not get run.
const DirEnv = "AVA_ENGINE_DIR"

// LegacyDirEnv is DirEnv's name before the 0.6.0 rename, still honoured so
// an existing override keeps working. Remove in 0.7.0.
const LegacyDirEnv = "LOCAL_WHISPER_ENGINE_DIR"

// DefaultDir is where `ava setup` materializes the bundle: beside
// the config the menu bar app and the CLI already share, rather than a second
// application-data location.
func DefaultDir() (string, error) {
	if v := os.Getenv(DirEnv); v != "" {
		return v, nil
	}
	if v := os.Getenv(LegacyDirEnv); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Application Support", "ava", "engine"), nil
}

// InstalledScript returns the path to a materialized control script, and
// whether one is actually there. Callers use it to fall back when the repo's
// scripts/ directory is not reachable.
func InstalledScript() (string, bool) {
	dir, err := DefaultDir()
	if err != nil {
		return "", false
	}
	path := filepath.Join(dir, ScriptRelPath)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return path, true
	}
	return "", false
}

// Materialize writes the bundle into dir, overwriting what is there. It
// overwrites rather than skipping existing files so that an upgraded binary
// replaces an older bundle: a stale server.py against a newer client is the
// kind of mismatch this repo has already paid for once.
//
// It returns the number of files written.
func Materialize(dir string) (int, error) {
	var written int
	err := fs.WalkDir(bundle, "files", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel("files", path)
		if err != nil {
			return err
		}
		data, err := bundle.ReadFile(path)
		if err != nil {
			return err
		}
		out := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
			return err
		}
		// embed.FS does not preserve the executable bit, so it is restored
		// here. Without it the control script materializes unrunnable and the
		// failure surfaces at `engine start`, nowhere near this code.
		mode := os.FileMode(0644)
		if filepath.Ext(rel) == ".sh" {
			mode = 0755
		}
		if err := os.WriteFile(out, data, mode); err != nil {
			return err
		}
		// WriteFile does not chmod a file that already exists.
		if err := os.Chmod(out, mode); err != nil {
			return err
		}
		written++
		return nil
	})
	if err != nil {
		return written, fmt.Errorf("materialize engine bundle into %s: %w", dir, err)
	}
	return written, nil
}
