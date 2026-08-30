// Package audio holds the sox audio format shared across this repo:
// recording, the transcription engines, and anything else reading the
// recorded wav must agree on it.
package audio

// Target format sox is asked to produce/consume throughout this repo.
const (
	SampleRateHz = "16000"
	Channels     = "1"
)
