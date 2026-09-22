package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/iksnerd/ava/internal/speaker"
	"github.com/iksnerd/ava/internal/voiceconfig"
	"github.com/iksnerd/ava/pkg/mlx"
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
		Example: "  ava speak \"build finished\"\n" +
			"  git log -1 --format=%s | ava speak\n" +
			"  ava speak --voice bf_emma --speed 0.9 \"slower, british\"",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			text, err := readSpeakText(args, cmd.InOrStdin())
			if err != nil {
				return err
			}
			if err := validateVoice(voice); err != nil {
				return err
			}
			if voiceconfig.Muted() {
				fmt.Fprintf(cmd.ErrOrStderr(), "⚠️  %s\n", mutedNotice)
			}
			if async {
				return spawnDetached(detachedSpeakArgs(text, voice, speed, serverURL))
			}
			return speaker.New(serverURL).Speak(text, speaker.Options{
				Voice: voice,
				Speed: speed,
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&voice, "voice", "", "Kokoro voice id, or several comma-separated to blend as an average (default: the configured voice)")
	flags.Float64Var(&speed, "speed", 0, "Speech rate multiplier (default: the configured speed)")
	flags.BoolVar(&async, "async", false, "Return immediately instead of waiting for playback to finish")
	flags.StringVar(&serverURL, "server-url", "", fmt.Sprintf("mlx-engine base URL (default %s)", mlx.DefaultServerURL))

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

// validateVoice rejects an unknown voice id instead of letting the engine do
// it. The engine's rejection is indistinguishable from the engine being down,
// so internal/speaker falls back to macOS `say` — meaning a typo in --voice
// used to speak the whole message in a completely different voice and exit 0.
// The ids are a closed set this binary already ships for shell completion.
//
// An empty value means "use the configured voice" and is left alone. Two
// comma-separated ids are a blend, so each side is checked.
func validateVoice(voice string) error {
	if voice == "" {
		return nil
	}
	known := make(map[string]bool, len(voiceconfig.Voices()))
	for _, v := range voiceconfig.Voices() {
		known[v.ID] = true
	}
	for _, id := range strings.Split(voice, ",") {
		id = strings.TrimSpace(id)
		if id == "" {
			return fmt.Errorf("invalid --voice %q: empty id in the blend", voice)
		}
		if !known[id] {
			return fmt.Errorf(
				"unknown voice %q (run `ava voices` to list the %d available)",
				id, len(known))
		}
	}
	return nil
}

// detachedSpeakArgs is the child's command line for `speak --async`: the same
// speech, synchronous, with the text after "--" so text starting with a dash
// is not read as a flag.
func detachedSpeakArgs(text, voice string, speed float64, serverURL string) []string {
	args := []string{"speak"}
	if voice != "" {
		args = append(args, "--voice", voice)
	}
	if speed != 0 {
		args = append(args, "--speed", strconv.FormatFloat(speed, 'f', -1, 64))
	}
	if serverURL != "" {
		args = append(args, "--server-url", serverURL)
	}
	return append(args, "--", text)
}

// spawnDetached runs this binary with args in a new session, detached from
// the caller's terminal, and returns without waiting.
//
// `speak --async` used to start a goroutine inside this process and return,
// and the process exited with the goroutine still running: nothing was ever
// spoken. Async only works where the process outlives the call, which for
// the CLI means a second process. (The MCP server is long-lived, so it keeps
// speaker.Options.Async.) A variable so tests can see what would be launched
// without launching it.
var spawnDetached = func(args []string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find own binary for --async: %w", err)
	}
	cmd := exec.Command(self, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start background speaker: %w", err)
	}
	return cmd.Process.Release()
}
