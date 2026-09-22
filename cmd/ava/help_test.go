package main

import (
	"bytes"
	"strings"
	"testing"
)

// `ava --help` is the first thing a new user reads. It listed commands but
// never said where to start, and running plain `ava` starts recording.
func TestRootHelpSaysHowToGetStarted(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	for _, want := range []string{"ava setup", "Microphone", "Accessibility", "ava speak"} {
		if !strings.Contains(help, want) {
			t.Errorf("ava --help does not mention %q; a first-time user has no way in", want)
		}
	}
}
