package audio

import (
	"os"
	"os/exec"
)

// Target format sox is asked to produce/consume throughout this repo —
// recorder and processor must agree, since the recorder's output feeds the
// processor's input.
const (
	SampleRateHz = "16000"
	Channels     = "1"
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
	cmd := exec.Command("sox", p.InputPath, "-r", SampleRateHz, "-c", Channels, p.OutputPath, "norm", "-3")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}
