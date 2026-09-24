// Package audio holds the sox audio format shared across this repo:
// recording, the transcription engines, and anything else reading the
// recorded wav must agree on it.
package audio

import "github.com/iksnerd/ava/internal/protocol"

// Target format sox is asked to produce/consume throughout this repo.
const (
	SampleRateHz = "16000"
	Channels     = "1"
)

// TempDir is where ava puts the audio and transcripts it produces:
// ava's per-dictation wav (cleaned up on exit) and ava monitor's
// transcript logs (kept — see the README's privacy section). One spelling,
// because two spellings is how one of them ends up writing somewhere the
// other never cleans.
const TempDir = protocol.TempDir
