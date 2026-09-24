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
