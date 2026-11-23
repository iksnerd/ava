package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/iksnerd/ava/internal/protocol"
	"github.com/iksnerd/ava/internal/testutil"
)

// engineStub answers /health with the given body and counts /speak calls.
func engineStub(t *testing.T, health string) *atomic.Int32 {
	t.Helper()
	var speaks atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/speak" {
			speaks.Add(1)
			_, _ = w.Write([]byte("RIFF"))
			return
		}
		_, _ = w.Write([]byte(health))
	}))
	t.Cleanup(srv.Close)
	setupEngineURL = srv.URL
	t.Cleanup(func() { setupEngineURL = "" })
	return &speaks
}

const readyHealth = `{"protocol":"` + protocol.EngineProtocol + `"}`

// whisperSpy puts a whisper-cli on PATH that records the first bytes of the
// audio it was given. fail makes it exit 1.
func whisperSpy(t *testing.T, fail bool) (record string) {
	t.Helper()
	bin := t.TempDir()
	record = filepath.Join(t.TempDir(), "whisper-calls")
	body := "#!/bin/sh\nfor a; do case \"$a\" in *.wav) head -c 4 \"$a\" >> " + record + " ;; esac; done\n"
	if fail {
		body += "echo boom >&2\nexit 1\n"
	}
	testutil.WriteExec(t, filepath.Join(bin, "whisper-cli"), body)
	testutil.SetPath(t, bin, "/usr/bin", "/bin")
	home := t.TempDir()
	t.Setenv("HOME", home)
	model := filepath.Join(home, modelDirRel, baseModel)
	_ = os.MkdirAll(filepath.Dir(model), 0o755)
	_ = os.WriteFile(model, []byte("placeholder"), 0o644)
	return record
}

func TestWarmUpTranscribesASilentWAVAndSpeaksOnAReadyEngine(t *testing.T) {
	record := whisperSpy(t, false)
	speaks := engineStub(t, readyHealth)

	var out bytes.Buffer
	warmUp(&out, false)

	got, _ := os.ReadFile(record)
	if !strings.Contains(string(got), "RIFF") {
		t.Errorf("whisper-cli was not given a WAV: %q", got)
	}
	if speaks.Load() != 1 {
		t.Errorf("speak calls = %d, want 1", speaks.Load())
	}
	if out.Len() != 0 {
		t.Errorf("a clean warm-up printed %q", out.String())
	}
}

func TestWarmUpSkipsSynthesisWithSkipEngine(t *testing.T) {
	whisperSpy(t, false)
	speaks := engineStub(t, readyHealth)
	warmUp(&bytes.Buffer{}, true)
	if speaks.Load() != 0 {
		t.Errorf("speak calls = %d under skipEngine, want 0", speaks.Load())
	}
}

func TestWarmUpDoesNotSpeakToAStaleEngine(t *testing.T) {
	whisperSpy(t, false)
	speaks := engineStub(t, `{"status":"ok"}`)
	warmUp(&bytes.Buffer{}, false)
	if speaks.Load() != 0 {
		t.Errorf("speak calls = %d against a stale engine, want 0", speaks.Load())
	}
}

func TestWarmUpDoesNotStartADownEngine(t *testing.T) {
	whisperSpy(t, false)
	setupEngineURL = "http://127.0.0.1:1"
	t.Cleanup(func() { setupEngineURL = "" })
	var out bytes.Buffer
	warmUp(&out, false) // returns, prints nothing, starts nothing
	if out.Len() != 0 {
		t.Errorf("down engine printed %q", out.String())
	}
}

func TestWarmUpFailureIsOneLineAndNotAnError(t *testing.T) {
	whisperSpy(t, true)
	var out bytes.Buffer
	warmUp(&out, true)
	if n := strings.Count(out.String(), "\n"); n != 1 || !strings.Contains(out.String(), "Warm-up skipped") {
		t.Errorf("failure output = %q, want one Warm-up skipped line", out.String())
	}
}

func TestSetupWarmsUpUnlessNoWarmupAndNeverUnderCheck(t *testing.T) {
	skipOffAppleSilicon(t)
	for _, tc := range []struct {
		args      []string
		wantCalls bool
	}{
		{[]string{"setup", "--skip-engine"}, true},
		{[]string{"setup", "--skip-engine", "--no-warmup"}, false},
		{[]string{"setup", "--check", "--skip-engine"}, false},
	} {
		r := newSetupRig(t)
		r.addTools(t)
		r.addModel(t)
		bin := t.TempDir()
		record := filepath.Join(t.TempDir(), "calls")
		testutil.WriteExec(t, filepath.Join(bin, "whisper-cli"), "#!/bin/sh\necho x >> "+record+"\n")
		testutil.SetPath(t, bin, r.bin, "/usr/bin", "/bin")

		root := newRootCmd()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(tc.args)
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v\n%s", tc.args, err, out.String())
		}
		_, err := os.Stat(record)
		if (err == nil) != tc.wantCalls {
			t.Errorf("%v: whisper-cli called = %v, want %v", tc.args, err == nil, tc.wantCalls)
		}
	}
}

func TestSilentWAVHasAValidHeader(t *testing.T) {
	b := silentWAV(16000, 1e9)
	if len(b) != 44+32000 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		t.Errorf("bad WAV: len %d, head %q", len(b), b[:12])
	}
}

func TestHintStaleEngineOnlyForAStaleEngine(t *testing.T) {
	for _, tc := range []struct {
		health string
		want   bool
	}{{`{"status":"ok"}`, true}, {readyHealth, false}} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(tc.health))
		}))
		var out bytes.Buffer
		hintStaleEngine(&out, srv.URL)
		srv.Close()
		if got := strings.Contains(out.String(), "older than this ava"); got != tc.want {
			t.Errorf("health %s: hint = %v, want %v (%q)", tc.health, got, tc.want, out.String())
		}
	}
	var out bytes.Buffer
	hintStaleEngine(&out, "http://127.0.0.1:1")
	if out.Len() != 0 {
		t.Errorf("down engine printed %q", out.String())
	}
}
