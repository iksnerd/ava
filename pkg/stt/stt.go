// Package stt defines the shared shape of local-whisper's one-shot
// speech-to-text engines, so cmd/local-whisper can select one at runtime
// without branching on engine-specific types. pkg/stt/whisper implements
// Client; pkg/mlx also does (its Transcribe method), but lives outside
// this tree rather than nested under it — mlx-engine, the server it
// wraps, serves TTS (Kokoro's /speak) as much as STT, so pkg/mlx is a
// general mlx-engine client that happens to satisfy Client today, not an
// STT-specific package. pkg/stt/realtime is STT too (used by
// cmd/voice-monitor) but has a genuinely different, streaming shape
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
)

// Options are the parameters common to every engine's one-shot
// transcription call.
type Options struct {
	AudioPath     string
	OutputPath    string
	ContextPrompt string
	Language      string
}

// Client is satisfied by any one-shot transcription engine client
// (currently pkg/stt/whisper.Client and pkg/mlx.Client).
type Client interface {
	Transcribe(Options) (string, error)
}

// WriteOutputIfRequested writes text to opts.OutputPath if one was given —
// for debugging/caching, not something callers depend on. A write failure
// here doesn't invalidate an otherwise-successful transcription, so it's
// reported to stderr rather than returned as an error. Shared by every
// Client implementation instead of each repeating the same five lines.
func WriteOutputIfRequested(opts Options, text string) {
	if opts.OutputPath == "" {
		return
	}
	if err := os.WriteFile(opts.OutputPath, []byte(text), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️ Failed to write output file %s: %v\n", opts.OutputPath, err)
	}
}
