// Package transcribe defines the shared shape of local-whisper's
// transcription engines (pkg/whisper, pkg/mlxengine), so cmd/local-whisper
// can select one at runtime without branching on engine-specific types.
package transcribe

// Options are the parameters common to every engine's one-shot
// transcription call.
type Options struct {
	AudioPath     string
	OutputPath    string
	ContextPrompt string
	Language      string
}

// Client is satisfied by any one-shot transcription engine client
// (currently pkg/whisper.Client and pkg/mlxengine.Client).
type Client interface {
	Transcribe(Options) (string, error)
}
