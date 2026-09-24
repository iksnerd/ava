package speaker

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/iksnerd/ava/internal/ttsproto"
)

// Two players share one playback lock and one stop protocol: playLocked here,
// and scripts/play_locked.py behind speak.sh (the hooks and the menu bar). A
// stop fix applied to one of them only is a race the other one still has, so
// every case runs against both.
type player struct {
	name string
	// play blocks until playback ends. pid reports the process a stop would
	// kill if it named itself in the PID file instead of the player.
	play func(t *testing.T, lockPath, marker string) error
	pid  func() int
}

var playLockedScript = filepath.Join("..", "..", "scripts", "play_locked.py")

func players() []player {
	var scriptPID int
	return []player{
		{
			name: "Go",
			play: func(t *testing.T, lockPath, marker string) error {
				s := &Speaker{LockPath: lockPath}
				return s.playLocked("clip.wav", 0, marker+ttsproto.PlayPIDSuffix)
			},
			pid: os.Getpid,
		},
		{
			name: "play_locked.py",
			play: func(t *testing.T, lockPath, marker string) error {
				c := exec.Command("python3", playLockedScript, lockPath, "600",
					marker+ttsproto.PlayPIDSuffix, marker+ttsproto.StoppedSuffix,
					"afplay", "-v", "0", "clip.wav")
				c.Stderr = os.Stderr
				if err := c.Start(); err != nil {
					return err
				}
				scriptPID = c.Process.Pid
				return c.Wait()
			},
			pid: func() int { return scriptPID },
		},
	}
}

func eachPlayer(t *testing.T, run func(t *testing.T, p player)) {
	for _, p := range players() {
		t.Run(p.name, func(t *testing.T) { run(t, p) })
	}
}

func touchStopped(t *testing.T, marker string) {
	t.Helper()
	if err := os.WriteFile(marker+ttsproto.StoppedSuffix, nil, 0644); err != nil {
		t.Fatal(err)
	}
}

// A stop that lands after the last check and before the player's PID is
// published finds nothing to kill. The .stopped sidecar is written before any
// signal, so a player that finds it on getting the lock must not play.
func TestNoPlayerPlaysOnceStopped(t *testing.T) {
	eachPlayer(t, func(t *testing.T, p player) {
		afplayPID := stubAfplay(t)
		marker := filepath.Join(t.TempDir(), "marker")
		touchStopped(t, marker)

		if err := p.play(t, filepath.Join(t.TempDir(), "playback.lock"), marker); err != nil {
			t.Fatalf("play: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
		if _, err := os.Stat(afplayPID); err == nil {
			t.Error("a speak that was already stopped still started afplay")
		}
	})
}

// A speak queued behind another's playback has no player to kill yet, so it
// has to notice the stop itself and give up rather than play once the lock
// frees.
func TestNoQueuedPlayerPlaysOnceStopped(t *testing.T) {
	eachPlayer(t, func(t *testing.T, p player) {
		afplayPID := stubAfplay(t)
		lockPath := filepath.Join(t.TempDir(), "playback.lock")
		marker := filepath.Join(t.TempDir(), "marker")

		release := holdLock(t, lockPath)
		defer release()
		done := make(chan error, 1)
		go func() { done <- p.play(t, lockPath, marker) }()

		time.Sleep(100 * time.Millisecond)
		touchStopped(t, marker)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("a stopped speak kept waiting for the playback lock")
		}
		if _, err := os.Stat(afplayPID); err == nil {
			t.Error("a speak stopped while queued still started afplay")
		}
	})
}

// Every stop SIGTERMs the PID file's process. If a queued speak names itself
// there, a stop kills the wrapper — and, if it lands once the player has
// started, leaves that player orphaned and playing. The PID file may only
// ever name the player.
func TestNoQueuedPlayerNamesItself(t *testing.T) {
	eachPlayer(t, func(t *testing.T, p player) {
		stubAfplay(t)
		lockPath := filepath.Join(t.TempDir(), "playback.lock")
		marker := filepath.Join(t.TempDir(), "marker")
		pidFile := marker + ttsproto.PlayPIDSuffix

		release := holdLock(t, lockPath)
		done := make(chan error, 1)
		go func() { done <- p.play(t, lockPath, marker) }()

		for i := 0; i < 15; i++ {
			time.Sleep(20 * time.Millisecond)
			if data, err := os.ReadFile(pidFile); err == nil &&
				strings.TrimSpace(string(data)) == strconv.Itoa(p.pid()) {
				release()
				<-done
				t.Fatal("a queued speak wrote its own PID to the play pid file")
			}
		}
		release()
		if err := <-done; err != nil {
			t.Fatalf("play after the lock was released: %v", err)
		}
	})
}
