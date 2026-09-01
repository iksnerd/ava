// Package stt defines the shared shape of local-whisper's one-shot
// speech-to-text engines (pkg/stt/whisper, pkg/stt/mlx), so
// cmd/local-whisper can select one at runtime without branching on
// engine-specific types. pkg/stt/realtime is STT too (used by
// cmd/voice-monitor) but has a genuinely different, streaming shape
// (StreamRealtime/ListInputDevices) so it doesn't implement Client here.
//
// There is no sibling pkg/tts: nothing in this repo speaks Go to a TTS
// engine directly. All synthesis goes through scripts/speak.sh curling
// mlx-engine's /speak endpoint; internal/ttscontrol only cancels in-flight
// speech (a Go port of stop-speaking.sh's marker-file protocol), it never
// synthesizes anything.
package stt

// Options are the parameters common to every engine's one-shot
// transcription call.
type Options struct {
	AudioPath     string
	OutputPath    string
	ContextPrompt string
	Language      string
}

// Client is satisfied by any one-shot transcription engine client
// (currently pkg/stt/whisper.Client and pkg/stt/mlx.Client).
type Client interface {
	Transcribe(Options) (string, error)
}
