package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/iksnerd/ava/internal/enginedist"
)

// newEngineCmd groups start/stop/status for the local mlx-engine Kokoro TTS
// server. It shells out to
// scripts/mlx-engine-server.sh rather than reimplementing its PID-file
// locking and uvicorn process management in Go — that script already owns
// the mlx-engine/ Python venv it drives, so there is no independent Go-side
// state to keep in sync.
func newEngineCmd() *cobra.Command {
	var scriptPath string

	cmd := &cobra.Command{
		Use:   "engine",
		Short: "Manage the local mlx-engine Kokoro TTS server",
		// A group with no Run of its own prints help for anything typed after
		// it, exit 0, so `ava engine statu` looked like it worked.
		Args:          noArgsSuggesting,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.PersistentFlags().StringVar(&scriptPath, "script", "",
		"Path to the server control script (default: the repo's, else the one installed by ava setup)")

	// The same resolution speech auto-start uses, so the two never drive
	// different scripts.
	runScriptTo := func(out io.Writer, action string, extra ...string) error {
		script, err := enginedist.ResolveScript(scriptPath)
		if err != nil {
			return err
		}
		c := exec.Command("bash", append([]string{script, action}, extra...)...)
		c.Stdout = out
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("mlx-engine %s: %w (script: %s)", action, err, script)
		}
		return nil
	}
	runScript := func(action string, extra ...string) error {
		return runScriptTo(os.Stdout, action, extra...)
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
		newStatusCmd(runScriptTo),
	)

	return cmd
}

var statusPID = regexp.MustCompile(`PID: (\d+)`)

// newStatusCmd is `engine status`. The script owns the test for "running"
// (pid file, then /health), so this reads its answer rather than repeating
// the test: the line it prints names RUNNING or STOPPED, and the pid.
func newStatusCmd(run func(out io.Writer, action string, extra ...string) error) *cobra.Command {
	var check, asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show whether the mlx-engine server is running",
		Long: "Show whether the mlx-engine server is running. Exits 0 either way, so a\n" +
			"stopped engine is not an error; pass --check to get a non-zero exit when it\n" +
			"is not running.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !check && !asJSON {
				return run(cmd.OutOrStdout(), "status")
			}
			var buf bytes.Buffer
			if err := run(&buf, "status"); err != nil {
				return err
			}
			running := strings.Contains(buf.String(), "RUNNING")
			if asJSON {
				var pid any
				if m := statusPID.FindStringSubmatch(buf.String()); running && m != nil {
					pid, _ = strconv.Atoi(m[1])
				}
				b, _ := json.Marshal(struct {
					Running bool `json:"running"`
					PID     any  `json:"pid"`
				}{running, pid})
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
			}
			if check && !running {
				return silentExit{}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "Print nothing and exit non-zero when the server is not running")
	cmd.Flags().BoolVar(&asJSON, "json", false, `Print {"running":bool,"pid":N|null} on stdout`)
	return cmd
}
