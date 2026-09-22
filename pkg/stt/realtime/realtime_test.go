package realtime

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	fixturePython     = "testdata/bin/python3"
	fixturePythonFail = "testdata/bin-fail/python3"
)

func TestNewClient(t *testing.T) {
	client := NewClient("/path/to/python3", "/path/to/voxtral")

	if client.PythonPath != "/path/to/python3" {
		t.Errorf("expected PythonPath %q, got %q", "/path/to/python3", client.PythonPath)
	}
	if client.ScriptDir != "/path/to/voxtral" {
		t.Errorf("expected ScriptDir %q, got %q", "/path/to/voxtral", client.ScriptDir)
	}
}

func TestStreamRealtimeMissingPython(t *testing.T) {
	client := NewClient("/nonexistent/python3", "/nonexistent/voxtral")

	_, err := client.StreamRealtime(RealtimeOptions{}, func(RealtimeDelta) {})
	if err == nil {
		t.Error("expected error when venv python not found, got nil")
	}
}

func TestStreamRealtimeWithDeviceMissingPython(t *testing.T) {
	client := NewClient("/nonexistent/python3", "/nonexistent/voxtral")

	_, err := client.StreamRealtime(RealtimeOptions{Device: "BlackHole"}, func(RealtimeDelta) {})
	if err == nil {
		t.Error("expected error when venv python not found, got nil")
	}
}

// waitForDone collects every RealtimeDelta from a StreamRealtime session
// into events, up to and including the terminal "done" event, or fails the
// test if it doesn't arrive in time.
func waitForDone(t *testing.T, register func(onDelta func(RealtimeDelta))) []RealtimeDelta {
	t.Helper()

	var mu sync.Mutex
	var events []RealtimeDelta
	done := make(chan struct{})

	register(func(d RealtimeDelta) {
		mu.Lock()
		events = append(events, d)
		isDone := d.Event == "done"
		mu.Unlock()
		if isDone {
			close(done)
		}
	})

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the \"done\" event")
	}

	mu.Lock()
	defer mu.Unlock()
	return events
}

func TestStreamRealtimeDeliversAllEvents(t *testing.T) {
	client := NewClient(fixturePython, "testdata")

	var stop func() error
	events := waitForDone(t, func(onDelta func(RealtimeDelta)) {
		var err error
		stop, err = client.StreamRealtime(RealtimeOptions{}, onDelta)
		if err != nil {
			t.Fatalf("StreamRealtime() error = %v", err)
		}
	})
	// The fixture process is one-shot and typically exits on its own before
	// this reaches it; stop()'s job here is just to reap it, the same way
	// cmd/ava-monitor's real caller ignores stop()'s error on Ctrl+C.
	_ = stop()

	if len(events) != 4 {
		t.Fatalf("got %d events, want 4: %+v", len(events), events)
	}
	if events[0].Event != "ready" || events[0].Device != "fixture-mic" {
		t.Errorf("first event = %+v, want a ready event for fixture-mic", events[0])
	}
	if events[1].Event != "delta" || events[1].Text != "hello " {
		t.Errorf("second event = %+v, want delta %q", events[1], "hello ")
	}
	if events[2].Event != "delta" || events[2].Text != "world" {
		t.Errorf("third event = %+v, want delta %q", events[2], "world")
	}
	if events[3].Event != "done" {
		t.Errorf("last event = %+v, want a done event", events[3])
	}
}

func TestStreamRealtimePassesOptionsThrough(t *testing.T) {
	client := NewClient(fixturePython, "testdata")
	logPath := filepath.Join(t.TempDir(), "realtime.log")
	t.Setenv("REALTIME_LOG", logPath)

	var stop func() error
	waitForDone(t, func(onDelta func(RealtimeDelta)) {
		var err error
		stop, err = client.StreamRealtime(RealtimeOptions{
			Device:     "BlackHole",
			Engine:     "whisper",
			Model:      "custom-model",
			Language:   "bg",
			Diarize:    true,
			HighpassHz: 120,
		}, onDelta)
		if err != nil {
			t.Fatalf("StreamRealtime() error = %v", err)
		}
	})
	_ = stop()

	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read realtime.py invocation log: %v", err)
	}
	for _, want := range []string{
		"--device", "BlackHole",
		"--engine", "whisper",
		"--model", "custom-model",
		"--language", "bg",
		"--diarize",
		"--highpass-hz", "120",
	} {
		if !strings.Contains(string(log), want) {
			t.Errorf("realtime.py args = %q, want it to contain %q", log, want)
		}
	}
}

func TestStreamRealtimeStopReportsProcessFailure(t *testing.T) {
	client := NewClient(fixturePythonFail, "testdata")

	stop, err := client.StreamRealtime(RealtimeOptions{}, func(RealtimeDelta) {})
	if err != nil {
		t.Fatalf("StreamRealtime() error = %v, want nil (the process starts fine, it just exits non-zero)", err)
	}

	done := make(chan error, 1)
	go func() { done <- stop() }()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected stop() to surface the fixture process's non-zero exit, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stop() did not return in time")
	}
}

func TestListInputDevicesMissingPython(t *testing.T) {
	client := NewClient("/nonexistent/python3", "/nonexistent/voxtral")

	_, err := client.ListInputDevices()
	if err == nil {
		t.Error("expected error when venv python not found, got nil")
	}
}

func TestListInputDevicesSuccess(t *testing.T) {
	client := NewClient(fixturePython, "testdata")

	out, err := client.ListInputDevices()
	if err != nil {
		t.Fatalf("ListInputDevices() error = %v", err)
	}
	if !strings.Contains(out, "BlackHole") {
		t.Errorf("output = %q, want it to list the fixture BlackHole device", out)
	}
}

func TestListInputDevicesCommandFailure(t *testing.T) {
	client := NewClient(fixturePythonFail, "testdata")

	_, err := client.ListInputDevices()
	if err == nil || !strings.Contains(err.Error(), "voxtral list-devices failed") {
		t.Errorf("err = %v, want it to mention voxtral list-devices failed", err)
	}
}
