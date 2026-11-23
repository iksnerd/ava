// Package stt defines the shared shape of ava's one-shot
// speech-to-text engine. pkg/stt/whisper is the only implementation now:
// pkg/mlx used to be a second one, until measuring it removed the reason to
// keep it. The interface stays because pkg/stt/realtime is STT too (used by
// internal/monitor) but has a genuinely different, streaming shape
// (StreamRealtime/ListInputDevices) so it doesn't implement Client either.
//
// There is still no sibling pkg/tts, and there shouldn't be one:
// synthesis is pkg/mlx.Client.Speak (same server, same HTTP-client
// scaffolding), internal/speaker wraps it in the mute/activity-marker/
// playback-lock protocol scripts/speak.sh established, and
// internal/ttscontrol cancels speech in flight (a Go port of
// stop-speaking.sh's marker-file protocol).
package stt

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Options are the parameters common to every engine's one-shot
// transcription call.
type Options struct {
	AudioPath     string
	OutputPath    string
	ContextPrompt string
	Language      string
	// BeamSize is whisper.cpp's beam width. Zero leaves whisper-cli's own
	// default (5) in place; lowering it trades accuracy for speed.
	BeamSize int
	// Format is how the result is rendered: "" or "text" for the plain
	// transcript, or a caption format ("srt", "vtt", "json") that carries each
	// segment's start and end time. See ValidateFormat.
	Format string
}

// Formats are the values Options.Format takes. "text" is the default, so
// every existing caller keeps getting the plain transcript.
var Formats = []string{"text", "srt", "vtt", "json"}

// ValidateFormat rejects a format no engine renders. The CLI and the MCP tool
// both call it, so the two cannot disagree about what is accepted.
func ValidateFormat(format string) error {
	if format == "" {
		return nil
	}
	for _, f := range Formats {
		if f == format {
			return nil
		}
	}
	return fmt.Errorf("unknown format %q (choose one of %s)", format, strings.Join(Formats, ", "))
}

// WantsTimestamps reports whether the format needs segment times, which
// plain text does not.
func (o Options) WantsTimestamps() bool {
	return o.Format != "" && o.Format != "text"
}

// Client is satisfied by any one-shot transcription engine client
// (currently only pkg/stt/whisper.Client).
type Client interface {
	Transcribe(Options) (string, error)
}

// WriteOutputIfRequested writes text to opts.OutputPath if one was given and
// returns the write error, naming the path. Whether a failure matters is the
// caller's call: the dictation path writes a debug copy and only warns, while
// `ava transcribe --output` was asked for the file and exits non-zero.
func WriteOutputIfRequested(opts Options, text string) error {
	if opts.OutputPath == "" {
		return nil
	}
	if err := os.WriteFile(opts.OutputPath, []byte(text), 0644); err != nil {
		return fmt.Errorf("cannot write output file %s: %w", opts.OutputPath, err)
	}
	return nil
}

var (
	bracketed = regexp.MustCompile(`\[[^\]]*\]`)
	musicNote = strings.NewReplacer("♪", "", "♫", "")
)

// IsNonSpeech reports whether whisper's output is nothing but non-speech
// markers: music notes and bracketed tags such as [BLANK_AUDIO] or [MUSIC].
// whisper-cli prints these for a tone or silence instead of nothing. Output
// that mixes a marker with words is speech and is not matched. Empty text is
// not a marker; callers test for that themselves.
func IsNonSpeech(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	rest := musicNote.Replace(bracketed.ReplaceAllString(text, ""))
	return strings.TrimSpace(rest) == ""
}
