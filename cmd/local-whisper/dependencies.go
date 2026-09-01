package main

import (
	"fmt"
	"net/http"
	"os/exec"
	"time"
)

// voxtralHealthURL is checkDependencies' default health-check endpoint for
// the mlx-engine server backing the voxtral engine.
const voxtralHealthURL = "http://127.0.0.1:8765/health"

// checkDependencies verifies the external tools/services engine needs are
// present before recording starts: sox always, whisper-cli for the whisper
// engine, and a reachable mlx-engine server (at voxtralHealthURL) for
// voxtral.
func checkDependencies(engine, voxtralHealthURL string) error {
	deps := []string{"sox"}
	if engine == "whisper" {
		deps = append(deps, "whisper-cli")
	}

	for _, dep := range deps {
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

	if engine == "voxtral" {
		// Quick HTTP GET to see if the server and model are up
		client := &http.Client{Timeout: 1 * time.Second}
		res, err := client.Get(voxtralHealthURL)
		if err != nil {
			return fmt.Errorf("voxtral server is not running or model failed to load. Start it by running: local-whisper engine start")
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return fmt.Errorf("voxtral server is not running or model failed to load. Start it by running: local-whisper engine start")
		}
	}

	return nil
}
