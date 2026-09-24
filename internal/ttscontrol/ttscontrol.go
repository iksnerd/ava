// Package ttscontrol lets other ava commands cancel any Claude
// Voice TTS currently in flight — needed before
// opening the mic, since speech still playing goes out the speakers and
// back in through the mic while sox is capturing.
//
// This is a Go port of scripts/stop-speaking.sh's marker-file protocol, not
// a wrapper around the script itself: ava is typically installed
// to ~/.local/bin standalone (see `make install-bin`), so it can't assume
// the repo's scripts/ directory is reachable. The protocol's constants live
// in internal/ttsproto, which is also where the test pinning them to the
// bash side lives.
package ttscontrol

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/iksnerd/ava/internal/ttsproto"
)

// ActivityDirEnv is re-exported from internal/ttsproto, which owns the
// protocol, so existing callers and tests keep one import.
const ActivityDirEnv = ttsproto.ActivityDirEnv

// stopWaitTimeout caps how long StopSpeaking waits for an in-flight speak to
// actually exit after being signaled. Killing a process is near-instant, so
// this is a safety cap against a wedged player, not a normal wait.
const stopWaitTimeout = 500 * time.Millisecond

// StopSpeaking cancels any TTS currently in flight (synthesis or playback)
// and waits briefly for it to actually stop before returning, so a caller
// about to record from the mic doesn't pick up speech still coming out the
// speakers. Safe to call when nothing is speaking.
func StopSpeaking() {
	stop(ttsproto.ActivityDir())
}

func stop(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	var stopped bool
	for _, entry := range entries {
		name := entry.Name()
		if ttsproto.IsSidecar(name) {
			continue
		}
		stopped = true
		marker := filepath.Join(dir, name)

		// Tells the speak this was a deliberate stop, not a failure, so it
		// neither plays nor falls back to `say`.
		_ = os.WriteFile(marker+ttsproto.StoppedSuffix, nil, 0644)

		for _, suffix := range ttsproto.PIDSuffixes() {
			killFromPidFile(marker + suffix)
		}
	}

	if !stopped {
		return
	}

	deadline := time.Now().Add(stopWaitTimeout)
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func killFromPidFile(pidFile string) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	// Never our own PID: a stop runs inside `ava`, and a pid file naming
	// this process would make it SIGTERM itself.
	if err != nil || pid == os.Getpid() {
		return
	}
	_ = syscall.Kill(pid, syscall.SIGTERM)
}
