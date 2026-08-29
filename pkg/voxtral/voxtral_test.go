package voxtral

import (
	"testing"
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

func TestTranscribeOptions(t *testing.T) {
	opts := TranscribeOptions{
		AudioPath: "/tmp/audio.wav",
		Language:  "en",
	}

	if opts.AudioPath != "/tmp/audio.wav" {
		t.Errorf("expected AudioPath /tmp/audio.wav, got %s", opts.AudioPath)
	}
	if opts.Language != "en" {
		t.Errorf("expected Language en, got %s", opts.Language)
	}
}

func TestTranscribeMissingPython(t *testing.T) {
	client := NewClient("/nonexistent/python3", "/nonexistent/voxtral")

	_, err := client.Transcribe(TranscribeOptions{AudioPath: "/tmp/audio.wav", Language: "en"})
	if err == nil {
		t.Error("expected error when venv python not found, got nil")
	}
}

func TestSpeakMissingPython(t *testing.T) {
	client := NewClient("/nonexistent/python3", "/nonexistent/voxtral")

	err := client.Speak(SpeakOptions{Text: "hello", OutputPath: "/tmp/out.wav"})
	if err == nil {
		t.Error("expected error when venv python not found, got nil")
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

func TestListInputDevicesMissingPython(t *testing.T) {
	client := NewClient("/nonexistent/python3", "/nonexistent/voxtral")

	_, err := client.ListInputDevices()
	if err == nil {
		t.Error("expected error when venv python not found, got nil")
	}
}
