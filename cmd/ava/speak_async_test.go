package main

import (
	"bytes"
	"strings"
	"testing"
)

// `speak --async` used to start a goroutine and return, and the process
// exited with it, so nothing was ever spoken. It must hand the speech to a
// detached child that outlives this process, with the same text and voice,
// and without --async (or the child would do the same thing again).
func TestSpeakAsyncHandsOffToADetachedChild(t *testing.T) {
	var launched []string
	orig := spawnDetached
	spawnDetached = func(args []string) error { launched = args; return nil }
	defer func() { spawnDetached = orig }()

	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"speak", "--async", "--voice", "af_heart", "--speed", "1.2", "--", "-dash leading text"})
	if err := root.Execute(); err != nil {
		t.Fatalf("speak --async: %v", err)
	}

	if launched == nil {
		t.Fatal("speak --async launched nothing; the speech dies with this process")
	}
	got := strings.Join(launched, " ")
	if launched[0] != "speak" {
		t.Errorf("child args %q, want the speak subcommand first", got)
	}
	for _, want := range []string{"--voice af_heart", "--speed 1.2", "-- -dash leading text"} {
		if !strings.Contains(got, want) {
			t.Errorf("child args %q, missing %q", got, want)
		}
	}
	if strings.Contains(got, "--async") {
		t.Errorf("child args %q still carry --async, so the child would hand off again and never speak", got)
	}
}

// Validation still happens in the foreground, where the caller can see it.
func TestSpeakAsyncRejectsAnUnknownVoiceBeforeLaunching(t *testing.T) {
	orig := spawnDetached
	spawnDetached = func([]string) error { t.Error("launched a child for an unknown voice"); return nil }
	defer func() { spawnDetached = orig }()

	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"speak", "--async", "--voice", "bf_emmma", "hi"})
	if err := root.Execute(); err == nil {
		t.Error("speak --async accepted an unknown voice")
	}
}
