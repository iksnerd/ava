package speaker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iksnerd/ava/internal/voiceconfig"
	"github.com/iksnerd/ava/pkg/mlx"
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

// "did not come up" alone gave no way to tell a typo'd --server-url from a
// broken engine, so the error names where it looked.
func TestAutoStartThatLeavesTheEngineDownNamesTheURL(t *testing.T) {
	s, _ := newTestSpeaker(t, &stubEngine{}, settingsWith(autoStartOn))
	s.Engine = mlx.NewClient("http://127.0.0.1:9")
	s.AutoStart = func(context.Context) error { return nil }

	err := s.ready(context.Background(), voiceconfig.Defaults())
	if err == nil || !strings.Contains(err.Error(), "did not come up") ||
		!strings.Contains(err.Error(), "http://127.0.0.1:9") {
		t.Errorf("err = %v, want did not come up naming the URL", err)
	}
}
