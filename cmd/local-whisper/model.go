package main

import "fmt"

const (
	baseModel = "ggml-base.en.bin"
	tinyModel = "ggml-tiny.en.bin"
)

// validateEngine checks that engine is one of the supported inference
// engines.
func validateEngine(engine string) error {
	if engine != "whisper" && engine != "voxtral" {
		return fmt.Errorf("invalid engine: %s (use 'whisper' or 'voxtral')", engine)
	}
	return nil
}

// validateModel checks the --model flag; it only constrains the whisper
// engine, which ships exactly two local models.
func validateModel(engine, model string) error {
	if engine == "whisper" && model != "base" && model != "tiny" {
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
