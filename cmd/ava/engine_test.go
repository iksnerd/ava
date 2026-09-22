package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `engine stop` writes engineAutoStart=false, a setting that outlives the
// command and silently routes every later hook through macOS `say`. Two
// sessions have started the engine to look at it and then had no way to put
// the machine back: undoing a throwaway stop meant starting the server again.
//
// These pin the two halves of the fix — the opt-out exists and reaches the
// script, and the script still says what it changed.

func TestStopOffersAWayToKeepAutoStart(t *testing.T) {
	engine := newEngineCmd()

	for _, c := range engine.Commands() {
		if c.Name() != "stop" {
			continue
		}
		if c.Flags().Lookup("keep-autostart") == nil {
			t.Fatal("engine stop has no --keep-autostart, so a temporary stop " +
				"cannot be undone except by starting the server again")
		}
		return
	}
	t.Fatal("engine has no stop subcommand")
}

func TestStopScriptHandlesKeepAutoStartAndSaysWhatItChanged(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "mlx-engine-server.sh"))
	if err != nil {
		t.Fatalf("read mlx-engine-server.sh: %v", err)
	}
	body := string(data)

	if !strings.Contains(body, "--keep-autostart") {
		t.Error("the control script does not handle --keep-autostart, so the CLI flag " +
			"is accepted and then silently ignored — worse than not offering it")
	}
	// The disclosure is the point: a stop that prints only "Server stopped"
	// is how a caller finds out about engineAutoStart later, from hooks that
	// have quietly gone silent.
	if !strings.Contains(body, "Hook auto-start is now OFF") {
		t.Error("the control script no longer says that it turned hook auto-start off")
	}
}
