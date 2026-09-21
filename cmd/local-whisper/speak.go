package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/iksnerd/local-whisper/internal/speaker"
	"github.com/iksnerd/local-whisper/internal/voiceconfig"
)

// newSpeakCmd speaks text through the same path the Claude Code hooks and
// the menu bar app use: Kokoro on the local mlx-engine, falling back to
// macOS `say`, under the global mute and the shared playback lock. It is
// the CLI counterpart of scripts/speak.sh, for a binary installed to
// ~/.local/bin with no repo checkout in sight.
func newSpeakCmd() *cobra.Command {
	var (
		voice     string
		speed     float64
		async     bool
		serverURL string
	)

	cmd := &cobra.Command{
		Use:   "speak [text...]",
		Short: "Speak text out loud via the local Kokoro TTS server",
		Long: "Speak text out loud on this Mac, on-device.\n\n" +
			"With no arguments it reads stdin, so a command's output can be piped\n" +
			"straight in without shell quoting. Blocks until playback finishes\n" +
			"unless --async is passed. Silent while the menu bar app's global\n" +
			"Mute is on, which it warns about rather than pretending to speak.",
		Example: "  local-whisper speak \"build finished\"\n" +
			"  git log -1 --format=%s | local-whisper speak\n" +
			"  local-whisper speak --voice bf_emma --speed 0.9 \"slower, british\"",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			text, err := readSpeakText(args, cmd.InOrStdin())
			if err != nil {
				return err
			}
			if voiceconfig.Muted() {
				fmt.Fprintf(cmd.ErrOrStderr(), "⚠️  %s\n", mutedNotice)
			}
			return speaker.New(serverURL).Speak(text, speaker.Options{
				Voice: voice,
				Speed: speed,
				Async: async,
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&voice, "voice", "", "Kokoro voice id, or two comma-separated to blend (default: the configured voice)")
	flags.Float64Var(&speed, "speed", 0, "Speech rate multiplier (default: the configured speed)")
	flags.BoolVar(&async, "async", false, "Return immediately instead of waiting for playback to finish")
	flags.StringVar(&serverURL, "server-url", "", "mlx-engine base URL (default http://127.0.0.1:8765)")

	// Shell completion for --voice: the ids are a closed set this binary
	// already knows, so there is no reason to make anyone look them up.
	_ = cmd.RegisterFlagCompletionFunc("voice",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			var out []string
			for _, v := range voiceconfig.Voices() {
				out = append(out, v.ID+"\t"+v.Description)
			}
			return out, cobra.ShellCompDirectiveNoFileComp
		})

	return cmd
}

// readSpeakText takes the text from the arguments, or from stdin when there
// are none. Speaking nothing is an error rather than a silent success:
// a hook that pipes in an empty transcript should say so, not look like it
// worked.
func readSpeakText(args []string, stdin io.Reader) (string, error) {
	if len(args) > 0 {
		return strings.Join(args, " "), nil
	}

	data, err := io.ReadAll(stdin)
	if err != nil {
		return "", fmt.Errorf("failed to read text from stdin: %w", err)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", fmt.Errorf("nothing to speak: pass the text as arguments or pipe it in")
	}
	return text, nil
}
