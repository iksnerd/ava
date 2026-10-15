package main

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/iksnerd/ava/internal/buildinfo"
	"github.com/iksnerd/ava/internal/selfupdate"
)

func newUpdateCmd() *cobra.Command {
	var source string
	var force bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update ava to the latest release",
		Long: "Download the latest release for this Mac, check it against the release's\n" +
			"published SHA-256 checksum, and replace the running ava with it. Nothing on\n" +
			"disk changes unless the checksum matches.\n\n" +
			"A binary built from source (`dev`, or a commit after a tag) is not replaced\n" +
			"unless you pass --force. The Kokoro engine is embedded in the binary, so\n" +
			"updating ava updates it; run `ava setup` again only if a release says to.\n\n" +
			"Source: --source, then AVA_DOWNLOAD_BASE, then the GitHub release. A source is\n" +
			"a URL that serves checksums.txt and the archives, to try a build before it\n" +
			"is released.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			if resolved, err := filepath.EvalSymlinks(exe); err == nil {
				exe = resolved
			}
			if source == "" {
				source = os.Getenv("AVA_DOWNLOAD_BASE")
			}
			_, err = selfupdate.Run(cmd.Context(), selfupdate.Options{
				Current: buildinfo.Get(),
				Exe:     exe,
				GOOS:    runtime.GOOS,
				GOARCH:  runtime.GOARCH,
				Force:   force,
				Source:  source,
				Client:  &http.Client{Timeout: 5 * time.Minute},
				Out:     cmd.OutOrStdout(),
			})
			return err
		},
	}
	cmd.Flags().StringVar(&source, "source", "", "Base URL serving checksums.txt and the archives (default: AVA_DOWNLOAD_BASE, then the GitHub release)")
	cmd.Flags().BoolVar(&force, "force", false, "Reinstall the same version, go back to an older one, or replace a build made from source")
	return cmd
}
