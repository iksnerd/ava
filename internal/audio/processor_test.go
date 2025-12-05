package audio

import (
	"testing"
)

func TestNewProcessor(t *testing.T) {
	inputPath := "/tmp/input.wav"
	outputPath := "/tmp/output.wav"

	processor := NewProcessor(inputPath, outputPath)

	if processor.InputPath != inputPath {
		t.Errorf("expected InputPath %q, got %q", inputPath, processor.InputPath)
	}

	if processor.OutputPath != outputPath {
		t.Errorf("expected OutputPath %q, got %q", outputPath, processor.OutputPath)
	}
}

func TestProcessorNormalizePaths(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{"/tmp/raw.wav", "/tmp/processed.wav"},
		{"./input.wav", "./output.wav"},
		{"/home/user/audio.wav", "/home/user/normalized.wav"},
	}

	for _, tc := range testCases {
		processor := NewProcessor(tc.input, tc.output)

		if processor.InputPath != tc.input {
			t.Errorf("expected input %q, got %q", tc.input, processor.InputPath)
		}

		if processor.OutputPath != tc.output {
			t.Errorf("expected output %q, got %q", tc.output, processor.OutputPath)
		}
	}
}
