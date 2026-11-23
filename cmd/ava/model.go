package main

import (
	"fmt"

	"github.com/iksnerd/ava/pkg/mlx"
)

const (
	baseModel = "ggml-base.en.bin"
	tinyModel = "ggml-tiny.en.bin"
)

// validateModel checks the --model flag against the two local whisper.cpp
// models this ships with.
func validateModel(model string) error {
	if model != "base" && model != "tiny" {
		return fmt.Errorf("invalid model: %s (use 'base' or 'tiny')", model)
	}
	return nil
}

// beamSizeUsage is --beam-size's help text, shared by dictation and
// `transcribe` so the two describe it identically.
const beamSizeUsage = "Beam width for decoding; lower is faster and less accurate (0 = whisper's default, 5)"

// Flag help text shared by more than one command, so they describe the flag
// identically. The rendered --help of each command is unchanged.
const (
	serverURLUsage = "mlx-engine base URL (default " + mlx.DefaultServerURL + ")"
	speedUsage     = "Speech rate multiplier (default: the configured speed)"
	languageUsage  = "Language code: en, es, fr, de, etc."
	modelSizeUsage = "Model size: base or tiny"
)

// validateBeamSize rejects a negative --beam-size. Zero means "not set", so
// whisper-cli keeps its own default.
func validateBeamSize(n int) error {
	if n < 0 {
		return fmt.Errorf("invalid beam size: %d (use a positive number, or 0 for whisper's default of 5)", n)
	}
	return nil
}

// selectModelFile maps the --model flag to the whisper.cpp model filename
// under ~/.local/share/whisper-cpp/.
func selectModelFile(modelName string) string {
	if modelName == "tiny" {
		return tinyModel
	}
	return baseModel
}
