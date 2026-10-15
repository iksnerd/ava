package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeEngine answers /health and /speak the way mlx-engine does, so the
// command runs end to end without Kokoro or a speaker.
func fakeEngine(t *testing.T, wav string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/speak", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte(wav))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func runSpeak(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"speak"}, args...))
	err := root.Execute()
	return out.String(), err
}

// `speak --out` saves the engine's WAV and makes no sound.
func TestSpeakOutWritesTheWav(t *testing.T) {
	srv := fakeEngine(t, "RIFFwav")
	out := filepath.Join(t.TempDir(), "scene1.wav")

	if _, err := runSpeak(t, "--server-url", srv.URL, "--out", out, "scene one"); err != nil {
		t.Fatalf("speak --out: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil || string(got) != "RIFFwav" {
		t.Fatalf("file = %q, %v; want the engine's bytes", got, err)
	}
}

// A background child would return before the file exists, so a script that
// reads it next would race it.
func TestSpeakOutRejectsAsync(t *testing.T) {
	orig := spawnDetached
	spawnDetached = func([]string) error { t.Error("launched a child for --out"); return nil }
	defer func() { spawnDetached = orig }()

	_, err := runSpeak(t, "--out", filepath.Join(t.TempDir(), "a.wav"), "--async", "hi")
	if err == nil || !strings.Contains(err.Error(), "--async") {
		t.Fatalf("err = %v, want a --async/--out conflict", err)
	}
}

// A missing directory is the caller's mistake; say so before synthesizing.
func TestSpeakOutRejectsAMissingDirectory(t *testing.T) {
	srv := fakeEngine(t, "RIFFwav")
	missing := filepath.Join(t.TempDir(), "nope", "a.wav")

	_, err := runSpeak(t, "--server-url", srv.URL, "--out", missing, "hi")
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("err = %v, want an error naming the missing directory", err)
	}
}
