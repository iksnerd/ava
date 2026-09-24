package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iksnerd/ava/internal/enginedist"
	"github.com/iksnerd/ava/internal/testutil"
)

func TestPathBudgetRejectsAnInstallDirThatWouldTruncate(t *testing.T) {
	long := "/" + strings.Repeat("x", 150)
	err := checkPathBudget(long)
	if err == nil {
		t.Fatal("a 150-character install dir was accepted; espeak would truncate its " +
			"data path and the server would die on a missing phontab")
	}
	// The message has to name the cause, because the failure it replaces
	// points at espeak and mentions neither length nor directory.
	for _, want := range []string{"too long", "phontab", enginedist.DirEnv} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal never mentions %q: %v", want, err)
		}
	}
}

func TestPathBudgetAcceptsAShortInstallDir(t *testing.T) {
	if err := checkPathBudget("/tmp/lw"); err != nil {
		t.Errorf("a short install dir was refused: %v", err)
	}
}

// The default location has to fit, or the out-of-the-box install fails for
// everyone. Measured rather than assumed: this recomputes it from DefaultDir
// so that moving the install directory cannot quietly blow the budget.
func TestDefaultInstallDirFitsTheEspeakBudget(t *testing.T) {
	t.Setenv(enginedist.DirEnv, "")
	dir, err := enginedist.DefaultDir()
	if err != nil {
		t.Skipf("no home directory available: %v", err)
	}
	if err := checkPathBudget(dir); err != nil {
		t.Errorf("the default install directory does not fit espeak's path budget "+
			"on this machine, so `ava setup` cannot work out of the box: %v", err)
	}
	// Leave some headroom: a longer username on another machine must also fit.
	slack := espeakPathBudget - (len(dir) + len(espeakDataSuffix))
	if slack < 16 {
		t.Errorf("only %d characters of headroom under espeak's budget; a user with a "+
			"longer home directory would fail. Shorten the default install path.", slack)
	}
	t.Logf("default install path uses %d of %d bytes (%d spare)",
		len(dir)+len(espeakDataSuffix), espeakPathBudget, slack)
}

func TestSetupIsRegisteredOnTheRootCommand(t *testing.T) {
	for _, c := range newRootCmd().Commands() {
		if c.Name() == "setup" {
			if c.Flags().Lookup("skip-engine") == nil {
				t.Error("setup has no --skip-engine, so the 1.2 GB engine cannot be declined")
			}
			return
		}
	}
	t.Fatal("the root command has no setup subcommand")
}

// The engine step needs uv, and setup never installed it: on a fresh Mac with
// Homebrew, `ava setup` got through the tools and the whisper model, then
// stopped with "install uv (https://docs.astral.sh/uv/) and re-run".
func TestSetupToolsInstallsUvWithTheOtherTools(t *testing.T) {
	bin := t.TempDir()
	calls := filepath.Join(t.TempDir(), "brew-calls")
	for name, body := range map[string]string{
		"sox":         "exit 0",
		"whisper-cli": "exit 0",
		"brew":        `echo "$@" >> ` + calls,
	} {
		os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body+"\n"), 0755)
	}
	testutil.SetPath(t, bin, "/usr/bin", "/bin")

	if err := setupTools(&bytes.Buffer{}); err != nil {
		t.Fatalf("setupTools: %v", err)
	}
	got, _ := os.ReadFile(calls)
	if strings.TrimSpace(string(got)) != "install uv" {
		t.Errorf("brew was run as %q, want only `install uv`", strings.TrimSpace(string(got)))
	}
}
