package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/iksnerd/ava/pkg/mlx"
)

// Live call transcripts were a second binary, ava-monitor, which the release
// shipped although it cannot run without a checkout, and which `make
// install-bin` never updated. They are `ava monitor` now.

func TestMonitorCommandsResolve(t *testing.T) {
	root := newRootCmd()
	monitor, _, err := root.Find([]string{"monitor"})
	if err != nil || monitor.Name() != "monitor" {
		t.Fatalf("ava monitor not wired: %v", err)
	}
	for _, f := range []string{"device", "engine", "stt-model", "language", "diarize", "highpass-hz", "port", "log"} {
		if monitor.Flags().Lookup(f) == nil {
			t.Errorf("ava monitor missing --%s", f)
		}
	}

	devices, _, err := root.Find([]string{"monitor", "devices"})
	if err != nil || devices.Name() != "devices" {
		t.Fatalf("ava monitor devices not wired: %v", err)
	}
	// devices talks to the same voxtral/realtime.py, so it takes the same flags.
	for _, f := range []string{"python", "voxtral-dir"} {
		if devices.InheritedFlags().Lookup(f) == nil {
			t.Errorf("ava monitor devices missing inherited --%s", f)
		}
	}
}

// It needs the Voxtral environment from a checkout, found relative to the
// working directory by default. A release install run from anywhere else
// fails, and the help has to say why before the user finds out the hard way.
func TestMonitorHelpSaysItNeedsTheVoxtralCheckout(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"monitor", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"make setup-voxtral", "--voxtral-dir"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("ava monitor --help does not mention %q", want)
		}
	}
}

// mlx-engine owns 127.0.0.1:8765 and is running whenever anything has spoken,
// so a monitor defaulting to the same port loses the bind.
func TestMonitorDefaultPortDoesNotCollideWithMlxEngine(t *testing.T) {
	monitor, _, err := newRootCmd().Find([]string{"monitor"})
	if err != nil {
		t.Fatal(err)
	}
	port := monitor.Flags().Lookup("port").DefValue
	if strings.HasSuffix(mlx.NewClient("").ServerURL, ":"+port) {
		t.Errorf("ava monitor --port defaults to %s, mlx-engine's port (%s)", port, mlx.NewClient("").ServerURL)
	}
}
