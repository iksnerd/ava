package ttscontrol

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/iksnerd/ava/internal/testutil"
	"github.com/iksnerd/ava/internal/ttsproto"
)

// A speak that was SIGKILLed never ran its deferred cleanup. Its marker must
// not make stop() wait out the full cap, and must be swept with its sidecars.
func TestStopRemovesMarkerOfDeadProcess(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, strconv.Itoa(testutil.DeadPID(t))+"-1700000000000000000")
	for _, p := range []string{marker, marker + ttsproto.PlayPIDSuffix, marker + ttsproto.StoppedSuffix} {
		if err := os.WriteFile(p, []byte("1"), 0644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}

	start := time.Now()
	stop(dir)
	if elapsed := time.Since(start); elapsed > stopWaitTimeout/2 {
		t.Errorf("stop() took %v on a dead marker, want it not to wait", elapsed)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("dead marker and sidecars still present: %v", entries)
	}
}

// A marker owned by a live process is still a speak in flight.
func TestStopKeepsLiveMarkerPID(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, strconv.Itoa(os.Getpid())+"-1700000000000000000")
	if err := os.WriteFile(marker, nil, 0644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	stop(dir)
	if _, err := os.Stat(marker + ttsproto.StoppedSuffix); err != nil {
		t.Errorf("live marker was not signalled stopped: %v", err)
	}
}
