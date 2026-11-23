package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/iksnerd/ava/internal/testutil"
)

var traceLine = regexp.MustCompile(`^trace  (.+?) +\d+ ms$`)

// traceNames returns the stage name of every trace line on stderr, in order.
func traceNames(t *testing.T, stderr string) []string {
	t.Helper()
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(stderr), "\n") {
		m := traceLine.FindStringSubmatch(line)
		if m == nil {
			t.Fatalf("line %q is not a trace line; stderr:\n%s", line, stderr)
		}
		names = append(names, m[1])
	}
	return names
}

func TestTranscribeTraceTimesEachStageOnStderrOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	model := filepath.Join(home, modelDirRel, baseModel)
	os.MkdirAll(filepath.Dir(model), 0755)
	os.WriteFile(model, []byte("placeholder"), 0644)
	os.WriteFile(filepath.Join(home, "fixture-whisper-stdout"), []byte("hello world\n"), 0644)
	os.WriteFile(filepath.Join(home, "fixture-whisper-stderr"), []byte(
		"whisper_print_timings:     load time =   120.50 ms\n"+
			"whisper_print_timings:   encode time =   200.00 ms /     1 runs (  200.00 ms per run)\n"+
			"whisper_print_timings:   decode time =    30.25 ms /     5 runs (   6.05 ms per run)\n"), 0644)
	testutil.PrependPath(t, "testdata/bin")
	clip := filepath.Join(t.TempDir(), "clip.wav")
	os.WriteFile(clip, append([]byte("RIFF\x00\x00\x00\x00WAVE"), make([]byte, 32)...), 0644)

	run := func(args ...string) (string, string) {
		root := newRootCmd()
		var out, errb bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errb)
		root.SetArgs(append([]string{"transcribe"}, args...))
		if err := root.Execute(); err != nil {
			t.Fatalf("transcribe %v: %v", args, err)
		}
		return out.String(), errb.String()
	}

	plain, plainErr := run(clip)
	traced, tracedErr := run("--trace", clip)

	if traced != plain {
		t.Errorf("stdout changed with --trace: %q vs %q", traced, plain)
	}
	if plainErr != "" {
		t.Errorf("stderr without --trace = %q, want empty", plainErr)
	}
	want := []string{"audio check", "model resolve", "whisper-cli", "whisper load", "whisper encode", "whisper decode", "output write", "total"}
	if got := traceNames(t, tracedErr); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("stages = %v, want %v", got, want)
	}
	if !strings.Contains(tracedErr, "whisper load") || !strings.Contains(tracedErr, "121 ms") {
		t.Errorf("want whisper's own load time (rounded to 121 ms) in:\n%s", tracedErr)
	}
}

func TestSpeakTraceTimesEachStageOnStderrOnly(t *testing.T) {
	srv := fakeEngine(t, "RIFFwav")
	out := filepath.Join(t.TempDir(), "a.wav")

	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"speak", "--trace", "--server-url", srv.URL, "--out", out, "hi"})
	if err := root.Execute(); err != nil {
		t.Fatalf("speak --trace: %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	want := []string{"settings load", "engine check", "synthesis", "total"}
	if got := traceNames(t, stderr.String()); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("stages = %v, want %v", got, want)
	}
}

func TestTraceTableAlignsAndEndsWithTotal(t *testing.T) {
	var b bytes.Buffer
	tr := newTracer(&b)
	tr.Add("whisper-cli", 312_400_000)
	tr.Add("audio check", 1_000_000)
	tr.Flush()
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[2], "trace  total") {
		t.Fatalf("lines = %q, want two stages then total", lines)
	}
	if lines[0] != "trace  whisper-cli  312 ms" && !strings.HasPrefix(lines[0], "trace  whisper-cli") {
		t.Errorf("line 0 = %q", lines[0])
	}
	if len(lines[0]) != len(lines[1]) || len(lines[1]) != len(lines[2]) {
		t.Errorf("lines are not aligned: %q", lines)
	}
}
