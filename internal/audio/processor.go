package audio

import (
	"os"
	"os/exec"
)

// Processor handles audio normalization and conversion
type Processor struct {
	InputPath  string
	OutputPath string
}

// NewProcessor creates a new audio processor
func NewProcessor(inputPath, outputPath string) *Processor {
	return &Processor{
		InputPath:  inputPath,
		OutputPath: outputPath,
	}
}

// Normalize normalizes audio with rate conversion to ensure compatibility
func (p *Processor) Normalize() error {
	cmd := exec.Command("sox", p.InputPath, "-r", "16000", "-c", "1", p.OutputPath, "norm", "-3")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}
