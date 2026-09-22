package main

import "testing"

func TestRootFlagsPresent(t *testing.T) {
	cmd := newRootCmd()
	for _, f := range []string{"context", "output", "dir", "model", "lang", "no-paste", "no-sound", "verbose", "quiet"} {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("root command missing --%s", f)
		}
	}
}

func TestEngineCommandsResolve(t *testing.T) {
	root := newRootCmd()
	for _, path := range [][]string{{"engine", "start"}, {"engine", "stop"}, {"engine", "status"}} {
		c, _, err := root.Find(path)
		if err != nil || c.Name() != path[len(path)-1] {
			t.Errorf("command %v not wired: %v", path, err)
		}
	}
}
