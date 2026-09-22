package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/iksnerd/ava/pkg/stt"
)

// newTranscribeCmd transcribes a WAV file that already exists, as opposed
// to the root command, which records one first. Same engines, same model
// selection (newTranscriber), so the two can't disagree.
func newTranscribeCmd() *cobra.Command {
	var (
		language string
		model    string
		output   string
		beamSize int
	)

	cmd := &cobra.Command{
		Use:   "transcribe <audio.wav>",
		Short: "Transcribe an existing WAV file on-device",
		Args:  cobra.ExactArgs(1),
		Long: "Transcribe a WAV file that already exists on disk, as opposed to the\n" +
			"bare `ava` command, which records one first.\n\n" +
			"Runs whisper.cpp locally and works on any Mac.",
		Example: "  ava transcribe meeting.wav\n" +
			"  ava transcribe --lang es clip.wav\n" +
			"  ava transcribe --output notes.txt meeting.wav\n" +
			"  ava transcribe --model tiny --beam-size 1 clip.wav",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkAudioFile(args[0]); err != nil {
				return err
			}
			if err := validateModel(model); err != nil {
				return err
			}
			if err := validateBeamSize(beamSize); err != nil {
				return err
			}

			transcriber, err := newTranscriber(model)
			if err != nil {
				return err
			}

			text, err := transcriber.Transcribe(stt.Options{
				AudioPath:  args[0],
				OutputPath: output,
				Language:   language,
				BeamSize:   beamSize,
			})
			if err != nil {
				return fmt.Errorf("transcription failed: %w", err)
			}
			if strings.TrimSpace(text) == "" {
				// whisper-cli exits 0 with empty output both for a genuinely
				// silent recording and for a file it could not decode at all —
				// an AIFF named .wav, a truncated download, a zero-byte file.
				// Reporting "no speech" for the second case sends the user off
				// to check their microphone and silence thresholds when the
				// real problem is the file. Distinguish them before concluding
				// the room was quiet.
				if err := checkAudioDecodable(args[0]); err != nil {
					return err
				}
				return fmt.Errorf("no speech detected in %s", args[0])
			}

			fmt.Fprintln(cmd.OutOrStdout(), text)
			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&language, "lang", "en", "Language code: en, es, fr, de, etc.")
	flags.StringVar(&model, "model", "base", "Model size: base or tiny")
	flags.StringVar(&output, "output", "", "Also write the transcript to this file")
	flags.IntVar(&beamSize, "beam-size", 0, beamSizeUsage)

	_ = cmd.MarkFlagFilename("output", "txt")
	_ = cmd.RegisterFlagCompletionFunc("model",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return []string{"base", "tiny"}, cobra.ShellCompDirectiveNoFileComp
		})

	// The positional is always an audio file.
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return []string{"wav"}, cobra.ShellCompDirectiveFilterFileExt
	}

	return cmd
}

// checkAudioFile fails early on a path whisper-cli can't read. Worth doing
// here rather than letting the engine answer: whisper-cli's reply to a
// missing file is its entire help screen, which buries "file not found"
// under a hundred lines of flags.
func checkAudioFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no such audio file: %s", path)
		}
		return fmt.Errorf("cannot read %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("not a file: %s is a directory", path)
	}
	return nil
}

// checkAudioDecodable reports why a file produced no transcript, when the
// reason is the file rather than silence. Only called once the engine has
// already returned nothing, so the cost of opening the file again is irrelevant
// and the payoff is not blaming the user's microphone for a bad download.
func checkAudioDecodable(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", path, err)
	}
	defer f.Close()

	// A RIFF/WAVE header is 44 bytes; anything shorter cannot be a WAV at all.
	var header [12]byte
	n, err := io.ReadFull(f, header[:])
	if err != nil && n < len(header) {
		return fmt.Errorf("%s is not readable audio: only %d bytes, too short to be a WAV", path, n)
	}

	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return fmt.Errorf(
			"%s is not a WAV file: it starts with %q, not a RIFF/WAVE header. "+
				"Convert it first, e.g. `sox in.<ext> -r 16000 -c 1 out.wav`",
			path, printableMagic(header[0:4]))
	}
	return nil
}

// printableMagic renders the leading bytes of a file for an error message
// without emitting control characters into the user's terminal.
func printableMagic(b []byte) string {
	out := make([]rune, 0, len(b))
	for _, c := range b {
		if c >= 0x20 && c < 0x7f {
			out = append(out, rune(c))
		} else {
			out = append(out, '.')
		}
	}
	return string(out)
}
