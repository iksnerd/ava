package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestUpdateIsACommandWithSourceAndForce(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"update", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--source", "--force", "AVA_DOWNLOAD_BASE", "checksum"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("ava update --help does not mention %q:\n%s", want, out.String())
		}
	}
}

func TestUpdateRefusesADevBuildBeforeAnyRequest(t *testing.T) {
	// The test binary reports "dev", so this also proves the command reads the
	// build's own version. The unreachable source would fail first if it did not.
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"update", "--source", "http://127.0.0.1:1/nothing"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "not a release build") {
		t.Fatalf("err = %v, want one saying this is not a release build", err)
	}
}
