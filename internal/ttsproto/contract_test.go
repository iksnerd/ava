package ttsproto

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// The marker protocol is spelled in four places this repo controls: here, two
// bash scripts, and the Swift menu bar app. Go cannot hand a constant to bash
// or Swift — an installed local-whisper has no scripts/ directory beside it,
// and the app ships as a bundle — so the copies stay, and these tests fail
// when they DISAGREE with the constants above.
//
// They deliberately assert agreement rather than asserting each file contains
// the right literal: a per-file test asserting "speak.sh says /tmp/ava-tts-active"
// passes happily after someone changes the Go constant and forgets the script,
// which is the exact failure the protocol keeps producing.
//
// Adding a constant to this package means adding its readers here.

// assign matches `NAME="value"` in a bash script, the one form these scripts use.
func bashAssign(t *testing.T, script, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "scripts", script)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	re := regexp.MustCompile(fmt.Sprintf(`(?m)^%s="([^"]*)"`, regexp.QuoteMeta(name)))
	m := re.FindSubmatch(data)
	if m == nil {
		// Not "the value changed" but "the variable is gone" — a rename in the
		// script that this test would otherwise report as an empty mismatch.
		t.Fatalf("%s no longer assigns %s= at the start of a line; "+
			"if it was renamed, rename it here too", path, name)
	}
	return string(m[1])
}

func swiftLet(t *testing.T, file, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "AvaMenuBar", "Sources", "AvaMenuBar", file)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	re := regexp.MustCompile(fmt.Sprintf(`let\s+%s\s*=\s*"([^"]*)"`, regexp.QuoteMeta(name)))
	m := re.FindSubmatch(data)
	if m == nil {
		t.Fatalf("%s no longer declares let %s = \"...\"; "+
			"if it was renamed, rename it here too", path, name)
	}
	return string(m[1])
}

func TestActivityDirAgreesAcrossRuntimes(t *testing.T) {
	// ActivityDir() honours the env override, so read the constant directly.
	want := defaultActivityDir

	for _, c := range []struct {
		what string
		got  string
	}{
		{"scripts/speak.sh ACTIVITY_DIR", bashAssign(t, "speak.sh", "ACTIVITY_DIR")},
		{"scripts/stop-speaking.sh ACTIVITY_DIR", bashAssign(t, "stop-speaking.sh", "ACTIVITY_DIR")},
		{"SpeechActivityMonitor.swift activityDir", swiftLet(t, "SpeechActivityMonitor.swift", "activityDir")},
	} {
		if c.got != want {
			t.Errorf("%s = %q, ttsproto.defaultActivityDir = %q — a speak from one "+
				"runtime is invisible to the others until these agree", c.what, c.got, want)
		}
	}
}

func TestLockPathAgreesWithSpeakScript(t *testing.T) {
	// Not cosmetic: speak.sh takes this lock via Python's fcntl.flock, which is
	// flock(2), the same lock internal/speaker takes. Two paths means two locks,
	// which means both play at once instead of queueing.
	if got := bashAssign(t, "speak.sh", "PLAYBACK_LOCK"); got != LockPath {
		t.Errorf("speak.sh PLAYBACK_LOCK = %q, ttsproto.LockPath = %q — "+
			"different paths are different locks, so playback stops queueing", got, LockPath)
	}
}

func TestSidecarSuffixesAppearInBothStopPaths(t *testing.T) {
	// stop-speaking.sh and internal/ttscontrol must skip the same sidecars.
	// A suffix one of them does not know about is treated as a marker, and a
	// stop then tries to signal the pid file's own name.
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "stop-speaking.sh"))
	if err != nil {
		t.Fatalf("read stop-speaking.sh: %v", err)
	}
	for _, suffix := range []string{SynthPIDSuffix, PlayPIDSuffix, StoppedSuffix} {
		if !regexp.MustCompile(regexp.QuoteMeta(suffix)).Match(data) {
			t.Errorf("stop-speaking.sh never mentions %q, which ttsproto treats as a "+
				"sidecar — the script will treat it as a marker and signal its name", suffix)
		}
	}
}
