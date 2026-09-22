package main

import (
	"fmt"
	"os/exec"
)

// checkDependencies verifies the external tools recording and transcription
// need are present before recording starts.
func checkDependencies() error {
	for _, dep := range []string{"sox", "whisper-cli"} {
		if _, err := exec.LookPath(dep); err != nil {
			switch dep {
			case "sox":
				return fmt.Errorf("sox is not installed. Run: brew install sox")
			case "whisper-cli":
				return fmt.Errorf("whisper-cli is not installed. Run: brew install whisper-cpp")
			default:
				return fmt.Errorf("%s is not installed", dep)
			}
		}
	}
	return nil
}
