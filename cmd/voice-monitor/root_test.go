package main

import "testing"

func TestRootFlagsPresent(t *testing.T) {
	cmd := newRootCmd()
	for _, f := range []string{"device", "engine", "stt-model", "language", "diarize", "highpass-hz", "port", "log"} {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("root command missing --%s", f)
		}
	}
	for _, f := range []string{"python", "voxtral-dir"} {
		if cmd.PersistentFlags().Lookup(f) == nil {
			t.Errorf("root command missing persistent --%s", f)
		}
	}
}

func TestDevicesCommandResolves(t *testing.T) {
	root := newRootCmd()
	c, _, err := root.Find([]string{"devices"})
	if err != nil || c.Name() != "devices" {
		t.Errorf("command devices not wired: %v", err)
	}
	// devices must inherit the root's persistent --python/--voxtral-dir flags.
	for _, f := range []string{"python", "voxtral-dir"} {
		if c.InheritedFlags().Lookup(f) == nil {
			t.Errorf("devices command missing inherited --%s", f)
		}
	}
}
