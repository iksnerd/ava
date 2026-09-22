package main

import (
	"github.com/spf13/cobra"

	"github.com/iksnerd/ava/internal/ttscontrol"
)

// newStopCmd cancels speech in flight, whoever started it — this session,
// another session's hooks, or the menu bar app's Read Aloud. The CLI
// counterpart of scripts/stop-speaking.sh and the menu bar Stop button.
func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "stop",
		Short:         "Stop any speech currently being synthesized or played",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Safe when nothing is speaking, so there is nothing to report
			// and no error to raise.
			ttscontrol.StopSpeaking()
			return nil
		},
	}
}
