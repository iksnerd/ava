package recording

import (
	"testing"
)

func TestNewRecorder(t *testing.T) {
	outputPath := "/tmp/audio.wav"
	playSound := true

	recorder := NewRecorder(outputPath, playSound)

	if recorder.OutputPath != outputPath {
		t.Errorf("expected OutputPath %q, got %q", outputPath, recorder.OutputPath)
	}

	if recorder.PlaySound != playSound {
		t.Errorf("expected PlaySound %v, got %v", playSound, recorder.PlaySound)
	}
}

func TestRecorderWithoutSound(t *testing.T) {
	recorder := NewRecorder("/tmp/test.wav", false)

	if recorder.PlaySound {
		t.Error("expected PlaySound to be false")
	}
}

func TestRecorderWithSound(t *testing.T) {
	recorder := NewRecorder("/tmp/test.wav", true)

	if !recorder.PlaySound {
		t.Error("expected PlaySound to be true")
	}
}
