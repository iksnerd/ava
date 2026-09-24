package main

import (
	"github.com/spf13/cobra"

	"github.com/iksnerd/ava/internal/monitor"
)

// newMonitorCmd serves a live call transcript. It lives in internal/monitor
// (an SSE server, its hub and page) rather than here; this is the wiring.
func newMonitorCmd() *cobra.Command {
	return monitor.NewCmd()
}
