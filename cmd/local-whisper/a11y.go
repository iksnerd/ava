package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/iksnerd/local-whisper/internal/a11y"
	"github.com/iksnerd/local-whisper/internal/speaker"
	"github.com/iksnerd/local-whisper/internal/voiceconfig"
)

// newA11yCmd renders a Chrome accessibility tree as screen-reader
// announcements and speaks them. The same work the MCP
// speak_accessibility_tree tool does, for a terminal: save a snapshot to a
// file (or pipe it in) and iterate on it without an MCP client in the loop.
func newA11yCmd() *cobra.Command {
	var (
		mode      string
		quiet     bool
		voice     string
		speed     float64
		serverURL string
	)

	cmd := &cobra.Command{
		Use:   "a11y [snapshot-file]",
		Short: "Speak a Chrome accessibility tree the way a screen reader announces it",
		Args:  cobra.MaximumNArgs(1),
		Long: "Render chrome-devtools MCP's take_snapshot output as screen-reader\n" +
			"announcements, print them with any findings, and speak them.\n\n" +
			"Reads the snapshot from a file, or from stdin when no file is given.\n" +
			"Findings cover what only shows up when a page is heard in order:\n" +
			"unlabeled controls, links that announce identically, skipped heading\n" +
			"levels. For the rule-based pass, run chrome-devtools' lighthouse_audit.",
		Example: "  local-whisper a11y snapshot.txt\n" +
			"  local-whisper a11y --mode headings --quiet snapshot.txt\n" +
			"  pbpaste | local-whisper a11y --mode links",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !a11y.IsMode(a11y.Mode(mode)) {
				return fmt.Errorf("unknown mode %q (use one of: %s)", mode, strings.Join(a11y.Modes(), ", "))
			}

			snapshot, err := readSnapshot(args, cmd.InOrStdin())
			if err != nil {
				return err
			}

			nodes := a11y.Parse(snapshot)
			if len(nodes) == 0 {
				return fmt.Errorf("no accessibility nodes in that input — pass chrome-devtools MCP's take_snapshot output (lines like `uid=1_0 RootWebArea \"Title\"`)")
			}

			utterances := a11y.Announce(nodes, a11y.Mode(mode))
			if len(utterances) == 0 {
				return fmt.Errorf("nothing to announce in %s mode — the page has no matching nodes", mode)
			}

			out := cmd.OutOrStdout()
			for _, u := range utterances {
				fmt.Fprintln(out, u)
			}
			// Findings are page-wide even when only part of the page was
			// announced, so say so — otherwise a headings pass reporting a
			// link problem reads as a bug.
			if findings := a11y.Findings(nodes); len(findings) > 0 {
				fmt.Fprintln(out, "\nFindings (whole page):")
				for _, f := range findings {
					fmt.Fprintf(out, "  - %s\n", f)
				}
			}

			if quiet {
				return nil
			}
			if voiceconfig.Muted() {
				fmt.Fprintf(cmd.ErrOrStderr(), "⚠️  %s\n", mutedNotice)
			}
			return speaker.New(serverURL).Speak(strings.Join(utterances, ". "), speaker.Options{Voice: voice, Speed: speed})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&mode, "mode", string(a11y.ModeReading), "What to announce: "+strings.Join(a11y.Modes(), ", "))
	flags.BoolVar(&quiet, "quiet", false, "Print the announcements without speaking them")
	flags.StringVar(&voice, "voice", "", "Kokoro voice id to narrate with (default: the configured voice)")
	flags.Float64Var(&speed, "speed", 0, "Speech rate multiplier (default: the configured speed)")
	flags.StringVar(&serverURL, "server-url", "", "mlx-engine base URL (default http://127.0.0.1:8765)")

	_ = cmd.RegisterFlagCompletionFunc("mode",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return a11y.Modes(), cobra.ShellCompDirectiveNoFileComp
		})

	return cmd
}

// readSnapshot takes the tree from a file argument, or from stdin.
func readSnapshot(args []string, stdin io.Reader) (string, error) {
	if len(args) > 0 {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return "", fmt.Errorf("read snapshot: %w", err)
		}
		return string(data), nil
	}

	data, err := io.ReadAll(stdin)
	if err != nil {
		return "", fmt.Errorf("read snapshot from stdin: %w", err)
	}
	return string(data), nil
}
