package main

import "fmt"

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

// selectModelFile maps the --model flag to the whisper.cpp model filename
// under ~/.local/share/whisper-cpp/.
func selectModelFile(modelName string) string {
	if modelName == "tiny" {
		return tinyModel
	}
	return baseModel
}
