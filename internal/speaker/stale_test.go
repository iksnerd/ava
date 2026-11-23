package speaker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/iksnerd/ava/internal/voiceconfig"
)

// A new speak sweeps markers left by a speak that was SIGKILLed.
func TestMarkActiveSweepsDeadProcessMarkers(t *testing.T) {
	s, _ := newTestSpeaker(t, &stubEngine{healthy: true}, voiceconfig.Defaults())
	if err := os.MkdirAll(s.ActivityDir, 0755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(s.ActivityDir, fmt.Sprintf("%d-1700000000000000000", cmd.Process.Pid))
	for _, p := range []string{stale, stale + ".play.pid", stale + ".stopped"} {
		if err := os.WriteFile(p, []byte("1"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	marker, err := s.markActive()
	if err != nil {
		t.Fatalf("markActive() error = %v", err)
	}
	defer clearMarker(marker)

	entries, _ := os.ReadDir(s.ActivityDir)
	if len(entries) != 1 || filepath.Join(s.ActivityDir, entries[0].Name()) != marker {
		t.Errorf("activity dir = %v, want only the new marker", entries)
	}
}
