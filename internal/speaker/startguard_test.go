package speaker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newGuard(t *testing.T) (*startGuard, *clock) {
	t.Helper()
	c := &clock{t: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)}
	return &startGuard{Path: filepath.Join(t.TempDir(), "failures"), Now: c.now, Max: 3, Window: 5 * time.Minute}, c
}

// RFC 0004 section 6: more than max restarts inside the window makes the
// plugin unavailable until the window passes. Three failures block; two do not.
func TestAGuardBlocksAfterMaxFailuresInTheWindow(t *testing.T) {
	g, _ := newGuard(t)
	for i := 1; i <= 2; i++ {
		g.Failed()
		if _, blocked := g.Blocked(); blocked {
			t.Fatalf("blocked after %d failures, want it to take 3", i)
		}
	}
	g.Failed()
	until, blocked := g.Blocked()
	if !blocked {
		t.Fatal("not blocked after 3 failures")
	}
	if want := g.Now().Add(5 * time.Minute); !until.Equal(want) {
		t.Errorf("blocked until %v, want %v: the window from the oldest failure", until, want)
	}
}

// Failures age out of the window one at a time.
func TestFailuresOlderThanTheWindowDoNotCount(t *testing.T) {
	g, c := newGuard(t)
	g.Failed()
	c.t = c.t.Add(2 * time.Minute)
	g.Failed()
	g.Failed()
	if _, blocked := g.Blocked(); !blocked {
		t.Fatal("not blocked after 3 failures inside the window")
	}

	// The first failure was 5 minutes ago: it has aged out, leaving two.
	c.t = c.t.Add(3 * time.Minute)
	if _, blocked := g.Blocked(); blocked {
		t.Error("still blocked when only 2 failures are inside the window")
	}
	// Just inside the window it still counts.
	c.t = c.t.Add(-time.Second)
	if _, blocked := g.Blocked(); !blocked {
		t.Error("the failure 4m59s old no longer counts")
	}
}

func TestASuccessClearsTheCount(t *testing.T) {
	g, _ := newGuard(t)
	g.Failed()
	g.Failed()
	g.Succeeded()
	g.Failed()
	g.Failed()
	if _, blocked := g.Blocked(); blocked {
		t.Error("blocked: the success did not reset the count, so 4 failures were counted as consecutive")
	}
	g.Failed()
	if _, blocked := g.Blocked(); !blocked {
		t.Error("not blocked after 3 failures since the success")
	}
}

// Hooks run as separate processes, so the count lives in a file: a second guard
// on the same path sees the first one's failures.
func TestTheCountIsSharedAcrossProcessesThroughTheFile(t *testing.T) {
	a, c := newGuard(t)
	b := &startGuard{Path: a.Path, Now: c.now, Max: 3, Window: 5 * time.Minute}
	a.Failed()
	b.Failed()
	a.Failed()
	if _, blocked := b.Blocked(); !blocked {
		t.Error("a second guard on the same file did not see the failures")
	}
}

func TestAGarbledFileIsIgnoredNotAnError(t *testing.T) {
	g, _ := newGuard(t)
	if err := os.WriteFile(g.Path, []byte("not a time\n\x00\x01\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, blocked := g.Blocked(); blocked {
		t.Error("blocked by a file with no valid entries")
	}
	g.Failed() // must still record
	raw, _ := os.ReadFile(g.Path)
	if strings.Count(string(raw), "\n") < 1 {
		t.Errorf("file = %q, want the failure recorded", raw)
	}
}

func TestAGuardWithNoFileOrAnUnwritablePathNeverBlocks(t *testing.T) {
	g, _ := newGuard(t)
	g.Path = filepath.Join(t.TempDir(), "no", "such", "dir", "failures")
	g.Failed() // cannot write: must not panic
	g.Failed()
	g.Failed()
	if _, blocked := g.Blocked(); blocked {
		t.Error("blocked with nowhere to keep the count: it should fail open, not lock speech out")
	}
	var nilGuard *startGuard
	if _, blocked := nilGuard.Blocked(); blocked {
		t.Error("a nil guard blocked")
	}
	nilGuard.Failed()
	nilGuard.Succeeded()
}

func TestTheFileStaysBounded(t *testing.T) {
	g, c := newGuard(t)
	for i := 0; i < 200; i++ {
		c.t = c.t.Add(10 * time.Minute) // each failure outlives the window
		g.Failed()
	}
	raw, _ := os.ReadFile(g.Path)
	if lines := strings.Count(string(raw), "\n"); lines > g.Max+1 {
		t.Errorf("%d entries kept, want old ones pruned", lines)
	}
}

// Failures that all fall inside the window are still capped at Max entries:
// the file cannot grow while the engine stays broken.
func TestManyFailuresInsideTheWindowAreStillBounded(t *testing.T) {
	g, _ := newGuard(t)
	for i := 0; i < 50; i++ {
		g.Failed()
	}
	raw, _ := os.ReadFile(g.Path)
	if lines := strings.Count(string(raw), "\n"); lines != g.Max {
		t.Errorf("%d entries kept after 50 failures, want %d", lines, g.Max)
	}
	if _, blocked := g.Blocked(); !blocked {
		t.Error("not blocked after 50 failures")
	}
}

// The file is written in order, but it is only a file: entries in any order give
// the same answer, because the block lifts when the oldest one ages out.
func TestAnOutOfOrderFileGivesTheSameAnswer(t *testing.T) {
	g, c := newGuard(t)
	var b strings.Builder
	for _, offset := range []time.Duration{0, 2 * time.Minute, time.Minute} {
		fmt.Fprintf(&b, "%d\n", c.t.Add(-offset).UnixNano())
	}
	if err := os.WriteFile(g.Path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	until, blocked := g.Blocked()
	if !blocked {
		t.Fatal("not blocked with 3 failures in the window")
	}
	if want := c.t.Add(-2 * time.Minute).Add(g.Window); !until.Equal(want) {
		t.Errorf("until %v, want %v (the oldest failure plus the window)", until, want)
	}
}
