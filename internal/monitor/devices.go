package monitor

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newDevicesCmd lists available input devices, sharing shared's --python/
// --voxtral-dir persistent flags with monitor.
func newDevicesCmd(shared *clientOptions) *cobra.Command {
	return &cobra.Command{
		Use:           "devices",
		Short:         "List available input devices and exit",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := shared.client().ListInputDevices()
			if err != nil {
				return err
			}
			fmt.Println(out)
			return nil
		},
	}
}
