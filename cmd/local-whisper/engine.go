package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// newEngineCmd groups start/stop/status for the local mlx-engine STT/TTS
// server (Kokoro TTS). It shells out to
// scripts/mlx-engine-server.sh rather than reimplementing its PID-file
// locking and uvicorn process management in Go — that script already owns
// the mlx-engine/ Python venv it drives, so there is no independent Go-side
// state to keep in sync.
func newEngineCmd() *cobra.Command {
	var scriptPath string

	cmd := &cobra.Command{
		Use:   "engine",
		Short: "Manage the local mlx-engine Kokoro TTS server",
	}
	cmd.PersistentFlags().StringVar(&scriptPath, "script", "scripts/mlx-engine-server.sh",
		"Path to the server control script (run local-whisper from the repo root, or override this)")

	runScript := func(action string, extra ...string) error {
		c := exec.Command("bash", append([]string{scriptPath, action}, extra...)...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("mlx-engine %s: %w (run from the repo root, or pass --script)", action, err)
		}
		return nil
	}

	newAction := func(use, short, action string) *cobra.Command {
		return &cobra.Command{
			Use:           use,
			Short:         short,
			Args:          cobra.NoArgs,
			SilenceUsage:  true,
			SilenceErrors: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runScript(action)
			},
		}
	}

	// stop is not a plain action: it also writes engineAutoStart=false, a
	// setting that outlives the command, so it needs a way to opt out.
	// Without one, a caller that started the engine to look at it has to
	// start the server again to undo a throwaway stop — the opposite of
	// putting the machine back as it found it.
	var keepAutoStart bool
	stop := &cobra.Command{
		Use:           "stop",
		Short:         "Stop the mlx-engine server, and stop hooks auto-starting it",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if keepAutoStart {
				return runScript("stop", "--keep-autostart")
			}
			return runScript("stop")
		},
	}
	stop.Flags().BoolVar(&keepAutoStart, "keep-autostart", false,
		"Stop the server but leave hook auto-start armed, for a temporary stop")

	cmd.AddCommand(
		newAction("start", "Start the mlx-engine server, and re-arm hook auto-start", "start"),
		stop,
		newAction("status", "Show whether the mlx-engine server is running", "status"),
	)

	return cmd
}
