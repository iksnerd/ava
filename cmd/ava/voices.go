package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/iksnerd/ava/internal/voiceconfig"
)

// newVoicesCmd lists what `speak --voice` accepts.
func newVoicesCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "voices",
		Short:         "List the Kokoro voices available to `speak`",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), formatVoices(voiceconfig.Load().Voice))
			return nil
		},
	}
}

// formatVoices renders the voice list. Shared with the MCP list_voices tool
// so the CLI and an agent asking the same question get the same answer.
func formatVoices(current string) string {
	var sb strings.Builder
	for _, v := range voiceconfig.Voices() {
		marker := ""
		if v.ID == current {
			marker = "  (current)"
		}
		fmt.Fprintf(&sb, "%-14s %s%s\n", v.ID, v.Description, marker)
	}
	sb.WriteString("\nAll of these ship in the same Kokoro model, so switching voices costs\n")
	sb.WriteString("no extra download. Pass several ids comma-separated to blend them as an\n")
	sb.WriteString("average, e.g. af_heart,af_sky.")
	return sb.String()
}
