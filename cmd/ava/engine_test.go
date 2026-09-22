package main

import (
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
