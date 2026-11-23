package main

import (
	"io"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
)

func TestRootFlagsPresent(t *testing.T) {
	cmd := newRootCmd()
	for _, f := range []string{"context", "output", "dir", "model", "lang", "no-paste", "no-sound", "verbose", "quiet"} {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("root command missing --%s", f)
		}
	}
}

func TestEngineCommandsResolve(t *testing.T) {
	root := newRootCmd()
	for _, path := range [][]string{{"engine", "start"}, {"engine", "stop"}, {"engine", "status"}} {
		c, _, err := root.Find(path)
		if err != nil || c.Name() != path[len(path)-1] {
			t.Errorf("command %v not wired: %v", path, err)
		}
	}
}

type fakeRec struct{ stops, kills int }

func (f *fakeRec) Stop() { f.stops++ }
func (f *fakeRec) Kill() { f.kills++ }

type interruptRun struct {
	out      string
	stops    int
	kills    int
	cleaned  bool
	exitCode int // -1: never exited
}

func driveInterrupts(t *testing.T, phase dictationPhase, sigs ...os.Signal) interruptRun {
	t.Helper()
	var p atomic.Int32
	p.Store(int32(phase))
	rec := &fakeRec{}
	res := interruptRun{exitCode: -1}
	h := newInterruptHandler(&p, rec, true, func() { res.cleaned = true }, func(c int) { res.exitCode = c })
	res.out = captureStdout(t, func() {
		for _, s := range sigs {
			h(s)
		}
	})
	res.stops, res.kills = rec.stops, rec.kills
	return res
}

func TestFirstSigintWhileRecordingStopsAndContinues(t *testing.T) {
	r := driveInterrupts(t, phaseRecording, syscall.SIGINT)
	if r.stops != 1 || r.kills != 0 || r.cleaned || r.exitCode != -1 {
		t.Errorf("got %+v, want one Stop and no cancel", r)
	}
	if !strings.Contains(r.out, "Recording stopped.") {
		t.Errorf("output %q missing stop message", r.out)
	}
}

func TestSecondSigintWhileRecordingCancels(t *testing.T) {
	r := driveInterrupts(t, phaseRecording, syscall.SIGINT, syscall.SIGINT)
	if r.stops != 1 || r.kills != 1 || !r.cleaned || r.exitCode != 0 {
		t.Errorf("got %+v, want Stop then Kill, cleanup, exit 0", r)
	}
	if !strings.Contains(r.out, "Recording cancelled.") {
		t.Errorf("output %q missing cancel message", r.out)
	}
}

func TestSigtermWhileRecordingCancelsAndKillsSox(t *testing.T) {
	r := driveInterrupts(t, phaseRecording, syscall.SIGTERM)
	if r.stops != 0 || r.kills != 1 || !r.cleaned || r.exitCode != 0 {
		t.Errorf("got %+v, want Kill, cleanup, exit 0", r)
	}
}

func TestSigintWhileTranscribingCancels(t *testing.T) {
	r := driveInterrupts(t, phaseTranscribing, syscall.SIGINT)
	if r.stops != 0 || !r.cleaned || r.exitCode != 0 || !strings.Contains(r.out, "Transcription cancelled.") {
		t.Errorf("got %+v, want transcription cancelled, exit 0", r)
	}
}

func TestSigintWhileDeliveringIsIgnored(t *testing.T) {
	r := driveInterrupts(t, phaseDelivering, syscall.SIGINT, syscall.SIGTERM)
	if r.out != "" || r.stops != 0 || r.kills != 0 || r.cleaned || r.exitCode != -1 {
		t.Errorf("got %+v, want nothing to happen", r)
	}
}

func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	f()
	os.Stdout = old
	w.Close()
	b, _ := io.ReadAll(r)
	return string(b)
}
