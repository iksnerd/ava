package main

import (
	"bytes"
	"strings"
	"testing"
)

// ava-monitor needs the Voxtral environment from a checkout, found relative to
// the working directory by default. A release install run from anywhere else
// fails, and the help has to say why before the user finds out the hard way.
func TestHelpSaysItNeedsTheVoxtralCheckout(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	for _, want := range []string{"make setup-voxtral", "--voxtral-dir"} {
		if !strings.Contains(help, want) {
			t.Errorf("ava-monitor --help does not mention %q", want)
		}
	}
}
