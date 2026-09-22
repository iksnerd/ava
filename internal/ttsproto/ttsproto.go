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
// The values themselves live in internal/protocol/protocol.json, which is
// rendered into Go, bash (scripts/protocol.sh) and Swift, so the runtimes can
// no longer spell them differently. internal/protocol/contract_test.go guards
// the remaining risk: a hand-written literal appearing beside a generated one.
package ttsproto

import (
	"os"
	"strings"

	"github.com/iksnerd/ava/internal/protocol"
)

// These re-export internal/protocol's generated constants. The package keeps
// its own names because they are the ones the rest of the repo reads, and
// because ActivityDir() has to be a function to honour the env override.
const (
	defaultActivityDir = protocol.ActivityDir
	ActivityDirEnv     = protocol.ActivityDirEnv
	LockPath           = protocol.PlaybackLock

	SynthPIDSuffix = protocol.SynthPIDSuffix
	PlayPIDSuffix  = protocol.PlayPIDSuffix
	StoppedSuffix  = protocol.StoppedSuffix
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
