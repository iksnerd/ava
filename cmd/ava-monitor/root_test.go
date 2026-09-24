package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iksnerd/ava/pkg/mlx"
)

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

// mlx-engine owns 127.0.0.1:8765, and CLAUDE.md's Ports convention says
// nothing else in this repo may reuse it. Both servers defaulting to one port
// means whichever starts second fails to bind — and the engine is running
// whenever anything has used --engine voxtral or spoken, so in practice it is
// ava-monitor that loses. The engine's port is read from pkg/mlx rather than
// written out here, so this can't pass against a stale copy of it.
func TestDefaultPortDoesNotCollideWithMlxEngine(t *testing.T) {
	port := newRootCmd().Flags().Lookup("port").DefValue

	engineURL := mlx.NewClient("").ServerURL
	if strings.HasSuffix(engineURL, ":"+port) {
		t.Errorf("ava-monitor defaults to port %s, which is mlx-engine's (%s) — pick another",
			port, engineURL)
	}
}

// With realtime.py dead, the monitor used to keep serving: the page stayed
// green, the log stopped growing, and nothing said why. It must exit instead,
// which closes the server and lets the page show "disconnected".
func TestWatchExitsWhenTranscriptionDies(t *testing.T) {
	python := filepath.Join(t.TempDir(), "python3")
	script := "#!/bin/sh\necho '{\"event\":\"ready\",\"device\":\"fixture\"}'\nexit 3\n"
	if err := os.WriteFile(python, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	opts := watchOptions{
		rootOptions: rootOptions{pythonPath: python, voxtralDir: t.TempDir()},
		logPath:     filepath.Join(t.TempDir(), "transcript.txt"),
		port:        port,
	}
	done := make(chan error, 1)
	go func() { done <- watch(opts) }()

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "exit status 3") {
			t.Errorf("watch() = %v, want an error carrying realtime.py's exit status", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watch() kept serving after realtime.py exited")
	}
}
