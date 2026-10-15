package speaker

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Default limits: after three failed auto-starts inside five minutes the engine
// is treated as unavailable until the oldest has aged out (RFC 0004 section 6,
// restart intensity). Starting costs a `uv sync` and up to two minutes of
// waiting, and every speak, hook included, would otherwise pay it again.
const (
	startFailuresMax    = 3
	startFailuresWindow = 5 * time.Minute
)

// startGuard remembers recent failed engine starts, so a speak does not retry
// one that keeps failing. The count lives in a file because hooks, the CLI and
// the MCP server are separate processes. It fails open: a file that cannot be
// read or written, or a nil guard, never blocks speech.
//
// Concurrent processes can lose an update to each other; the worst that costs
// is one more attempt than the limit.
type startGuard struct {
	Path   string
	Now    func() time.Time
	Max    int
	Window time.Duration
}

func (g *startGuard) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}

// recent returns the failures still inside the window, oldest first.
func (g *startGuard) recent() []time.Time {
	raw, err := os.ReadFile(g.Path)
	if err != nil {
		return nil
	}
	now := g.now()
	var out []time.Time
	for line := range strings.SplitSeq(string(raw), "\n") {
		n, err := strconv.ParseInt(strings.TrimSpace(line), 10, 64)
		if err != nil {
			continue // a garbled line is not a failure
		}
		at := time.Unix(0, n)
		if now.Sub(at) < g.Window {
			out = append(out, at)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

// Blocked reports whether starts have failed too often to try again, and until
// when.
func (g *startGuard) Blocked() (until time.Time, blocked bool) {
	if g == nil {
		return time.Time{}, false
	}
	recent := g.recent()
	if len(recent) < g.Max {
		return time.Time{}, false
	}
	// It lifts when enough of them have aged out to leave fewer than Max.
	return recent[len(recent)-g.Max].Add(g.Window), true
}

// Failed records a start that did not bring the engine up.
func (g *startGuard) Failed() {
	if g == nil {
		return
	}
	entries := append(g.recent(), g.now())
	if len(entries) > g.Max {
		entries = entries[len(entries)-g.Max:]
	}
	var b strings.Builder
	for _, at := range entries {
		fmt.Fprintf(&b, "%d\n", at.UnixNano())
	}
	// Written whole to a temp file and renamed in, so a reader never sees half.
	tmp := g.Path + "." + strconv.Itoa(os.Getpid())
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, g.Path); err != nil {
		_ = os.Remove(tmp)
	}
}

// Succeeded forgets the failures: the engine is up.
func (g *startGuard) Succeeded() {
	if g == nil {
		return
	}
	_ = os.Remove(g.Path)
}
