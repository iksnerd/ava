package main

import (
	"fmt"
	"io"
	"math"
	"time"
)

// traceUsage is the help for --trace on every command that has it.
const traceUsage = "Print the wall-clock time of each stage to stderr, with a total (stdout is unchanged)"

// tracer collects per-stage wall-clock times for `--trace` and prints them to
// the stream it was given, which is stderr: stdout stays the command's result.
// A nil *tracer ignores everything, so callers need no `if trace` around it.
type tracer struct {
	w      io.Writer
	start  time.Time
	stages []traceStage
}

type traceStage struct {
	name string
	d    time.Duration
}

func newTracer(w io.Writer) *tracer { return &tracer{w: w, start: time.Now()} }

// Add records d under name. A stage seen twice (the engine is checked before
// and after an auto-start) adds up and keeps its first position.
func (t *tracer) Add(name string, d time.Duration) {
	if t == nil {
		return
	}
	for i := range t.stages {
		if t.stages[i].name == name {
			t.stages[i].d += d
			return
		}
	}
	t.stages = append(t.stages, traceStage{name, d})
}

// Time returns a function that records the time since Time was called.
func (t *tracer) Time(name string) func() {
	start := time.Now()
	return func() { t.Add(name, time.Since(start)) }
}

// Flush prints one aligned line per stage, then the total since newTracer.
func (t *tracer) Flush() {
	if t == nil {
		return
	}
	rows := append(append([]traceStage(nil), t.stages...), traceStage{"total", time.Since(t.start)})
	ms := func(d time.Duration) string {
		return fmt.Sprintf("%.0f", math.Round(float64(d)/float64(time.Millisecond)))
	}
	nameWidth, msWidth := 0, 0
	for _, r := range rows {
		nameWidth = max(nameWidth, len(r.name))
		msWidth = max(msWidth, len(ms(r.d)))
	}
	for _, r := range rows {
		fmt.Fprintf(t.w, "trace  %-*s  %*s ms\n", nameWidth, r.name, msWidth, ms(r.d))
	}
}
