// Package ttsproto owns the marker-file protocol that every speaking and
// stopping path in this repo shares: scripts/speak.sh writes the markers,
// scripts/stop-speaking.sh and internal/ttscontrol cancel through them,
// internal/speaker joins them from Go, and AvaMenuBar's
// SpeechActivityMonitor polls the directory for its speaking indicator.
//
// It exists because those constants used to be spelled separately in each
// Go package that needed them, and separate spellings drift in *behaviour*
// before they drift in value: internal/ttscontrol honoured ActivityDirEnv
// while internal/speaker held a bare constant, so the variable that exists
// to keep tests off the real shared directory only ever redirected half the
// system. Both now resolve through ActivityDir().
//
// The bash and Swift halves of this protocol still spell these values
// themselves — Go cannot share a constant with either — so the copies that
// remain are pinned by a test rather than removed. See
// contract_test.go in this package: it reads the literals back out of
// scripts/speak.sh, scripts/stop-speaking.sh and VoiceSettings' sibling
// sources and fails when they disagree with the constants here.
package ttsproto

import (
	"os"
	"strings"
)

// defaultActivityDir holds one marker file per in-flight speak, present for
// the whole synth+playback duration. Mirrors speak.sh's ACTIVITY_DIR.
const defaultActivityDir = "/tmp/ava-tts-active"

// ActivityDirEnv overrides defaultActivityDir when set. Its only real use is
// pointing tests (here and in callers like internal/recording and
// internal/speaker) at an isolated directory, since they cannot otherwise
// exercise a stop without touching the real, shared activity dir — and any
// speak actually in flight on the machine running the test.
//
// The name keeps its TTSCONTROL_ prefix from when internal/ttscontrol was
// the only reader: it is an published escape hatch, and renaming it would
// silently stop working for anyone who had set it.
const ActivityDirEnv = "TTSCONTROL_ACTIVITY_DIR"

// LockPath is the exclusive flock(2) every playback holds, so a Go speak and
// a speak.sh speak queue behind each other instead of talking over one
// another. speak.sh takes the same lock via Python's fcntl.flock, which is
// flock(2) too. Mirrors speak.sh's PLAYBACK_LOCK.
const LockPath = "/tmp/ava-tts-playback.lock"

// Sidecar files written beside a marker. They are not themselves markers: a
// stop that treated them as one would try to signal the pid file's own name.
const (
	// SynthPIDSuffix names the process synthesizing, for a stop to signal.
	SynthPIDSuffix = ".synth.pid"
	// PlayPIDSuffix names the process playing, for a stop to signal.
	PlayPIDSuffix = ".play.pid"
	// StoppedSuffix tells speak.sh's wait_synth that a stop was deliberate,
	// so it does not treat the cancellation as a failure and fall back to `say`.
	StoppedSuffix = ".stopped"
)

// ActivityDir returns the directory holding in-flight speak markers,
// honouring ActivityDirEnv. Every reader of the protocol must go through
// this rather than the constant, or an isolated test stops being isolated.
func ActivityDir() string {
	if v := os.Getenv(ActivityDirEnv); v != "" {
		return v
	}
	return defaultActivityDir
}

// IsSidecar reports whether a directory entry is one of the sidecar files
// written beside a marker rather than a marker itself.
func IsSidecar(name string) bool {
	for _, suffix := range []string{SynthPIDSuffix, PlayPIDSuffix, StoppedSuffix} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

// PIDSuffixes are the sidecars naming a process a stop should signal, in the
// order a stop should signal them.
func PIDSuffixes() []string { return []string{SynthPIDSuffix, PlayPIDSuffix} }
