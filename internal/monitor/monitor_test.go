package monitor

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
		clientOptions: clientOptions{pythonPath: python, voxtralDir: t.TempDir()},
		logPath:       filepath.Join(t.TempDir(), "transcript.txt"),
		port:          port,
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

// Ctrl-C in a terminal reaches realtime.py and the monitor together, and
// realtime.py can exit first. watch used to take that exit for a crash and
// fail a deliberate stop with "transcription stopped". The stub reproduces the
// order: it exits, then the interrupt arrives. The background kill must not
// hold the stub's stdout open, or the stream only ends after the interrupt and
// the test passes whatever watch does.
func TestWatchTreatsCtrlCAsAStopEvenWhenTheTranscriberExitsFirst(t *testing.T) {
	python := filepath.Join(t.TempDir(), "python3")
	script := "#!/bin/sh\necho '{\"event\":\"ready\",\"device\":\"fixture\"}'\n(sleep 0.1; kill -INT $PPID) >/dev/null 2>&1 &\nexit 0\n"
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
		clientOptions: clientOptions{pythonPath: python, voxtralDir: t.TempDir()},
		logPath:       filepath.Join(t.TempDir(), "transcript.txt"),
		port:          port,
	}
	done := make(chan error, 1)
	go func() { done <- watch(opts) }()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("watch() after Ctrl-C = %v, want a clean stop", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watch() did not return after Ctrl-C")
	}
}
