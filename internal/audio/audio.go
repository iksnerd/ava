// Package audio holds the sox audio format shared across this repo:
// recording, the transcription engines, and anything else reading the
// recorded wav must agree on it.
package audio

// Target format sox is asked to produce/consume throughout this repo.
const (
	SampleRateHz = "16000"
	Channels     = "1"
)

// TempDir is where both binaries put the audio and transcripts they produce:
// local-whisper's per-dictation wav (cleaned up on exit) and voice-monitor's
// transcript logs (kept — see the README's privacy section). One spelling,
// because two spellings is how one of them ends up writing somewhere the
// other never cleans.
const TempDir = "/tmp/voice-input"
