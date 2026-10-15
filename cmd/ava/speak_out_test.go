package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iksnerd/ava/internal/protocol"
)

// fakeEngine answers /health and /speak the way mlx-engine does, so the
// command runs end to end without Kokoro or a speaker.
func fakeEngine(t *testing.T, wav string) *httptest.Server {
	t.Helper()
	srv, _ := fakeEngineRecording(t, wav)
	return srv
}

// fakeEngineRecording also returns the last request body /speak/timed saw.
func fakeEngineRecording(t *testing.T, wav string) (*httptest.Server, *map[string]any) {
	t.Helper()
	var timedBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/speak/timed", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&timedBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sample_rate":24000,"audio":"` + base64.StdEncoding.EncodeToString([]byte(wav)) + `",` +
			`"segments":[{"text":"Once upon a time.","start_ms":0,"end_ms":1500},{"text":"The end.","start_ms":1500,"end_ms":2000}]}`))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","protocol":"` + protocol.EngineProtocol + `"}`))
	})
	mux.HandleFunc("/speak", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte(wav))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &timedBody
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
	if err == nil || !strings.Contains(err.Error(), "async") {
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

// --timings writes the segments beside the WAV, and --split decides how fine
// they are: a sentence per segment sends the engine a sentence pattern.
func TestSpeakTimingsWritesTheWavAndTheSegments(t *testing.T) {
	srv, body := fakeEngineRecording(t, "RIFFtimed")
	dir := t.TempDir()
	out, timings := filepath.Join(dir, "a.wav"), filepath.Join(dir, "a.json")

	if _, err := runSpeak(t, "--server-url", srv.URL, "--out", out, "--timings", timings, "--split", "sentence", "Once upon a time. The end."); err != nil {
		t.Fatalf("speak --timings: %v", err)
	}
	if wav, _ := os.ReadFile(out); string(wav) != "RIFFtimed" {
		t.Errorf("wav = %q", wav)
	}
	got, _ := os.ReadFile(timings)
	want := `[{"start_ms":0,"end_ms":1500,"text":"Once upon a time."},{"start_ms":1500,"end_ms":2000,"text":"The end."}]`
	if strings.TrimSpace(string(got)) != want {
		t.Errorf("timings = %s, want %s", got, want)
	}
	if (*body)["split_pattern"] != `(?<=[.!?])\s+` {
		t.Errorf("engine got split_pattern %v, want the sentence pattern", (*body)["split_pattern"])
	}
}

func TestSpeakTimingsAndSplitNeedOut(t *testing.T) {
	for name, args := range map[string][]string{
		"timings": {"--timings", "a.json"},
		"split":   {"--split", "sentence"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := runSpeak(t, append(args, "hi")...)
			if err == nil || !strings.Contains(err.Error(), "--out") {
				t.Errorf("err = %v, want one saying it goes with --out", err)
			}
		})
	}
}

func TestSpeakRejectsAnUnknownSplit(t *testing.T) {
	_, err := runSpeak(t, "--out", filepath.Join(t.TempDir(), "a.wav"), "--split", "paragraph", "hi")
	if err == nil || !strings.Contains(err.Error(), "sentence") {
		t.Errorf("err = %v, want one listing the splits", err)
	}
}

func runSpeakWithStdin(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(append([]string{"speak"}, args...))
	err := root.Execute()
	return out.String(), err
}

// --out-dir reads the script the way speak always does, from stdin, and
// writes a WAV per line with a manifest. A file argument would be spoken as
// the words "lines.txt", so there is none.
func TestSpeakOutDirWritesOneWavPerLineAndAManifest(t *testing.T) {
	srv := fakeEngine(t, "RIFFscene")
	dir := filepath.Join(t.TempDir(), "scenes")

	if _, err := runSpeakWithStdin(t, "Once upon a time.\n\nThe end.\n", "--server-url", srv.URL, "--out-dir", dir); err != nil {
		t.Fatalf("speak --out-dir: %v", err)
	}
	for _, f := range []string{"001.wav", "002.wav"} {
		if got, err := os.ReadFile(filepath.Join(dir, f)); err != nil || string(got) != "RIFFscene" {
			t.Errorf("%s = %q, %v", f, got, err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("no manifest: %v", err)
	}
	var m []struct {
		Line       int    `json:"line"`
		File       string `json:"file"`
		DurationMS int    `json:"duration_ms"`
	}
	if err := json.Unmarshal(raw, &m); err != nil || len(m) != 2 || m[1].Line != 3 || m[1].File != "002.wav" || m[1].DurationMS != 2000 {
		t.Errorf("manifest = %s (%v)", raw, err)
	}
}

func TestSpeakOutDirExcludesTheSingleFileFlags(t *testing.T) {
	dir := t.TempDir()
	for name, args := range map[string][]string{
		"out":     {"--out", filepath.Join(dir, "a.wav")},
		"timings": {"--timings", filepath.Join(dir, "a.json")},
		"async":   {"--async"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := runSpeakWithStdin(t, "hi\n", append([]string{"--out-dir", filepath.Join(dir, "d")}, args...)...)
			if err == nil || strings.Contains(err.Error(), "unknown flag") || !strings.Contains(err.Error(), name) {
				t.Errorf("--out-dir with %s: err = %v, want a refusal that names %s", name, err, name)
			}
		})
	}
}

func TestSpeakOutDirOfAnEmptyScriptIsAnError(t *testing.T) {
	srv := fakeEngine(t, "RIFF")
	_, err := runSpeakWithStdin(t, "  \n", "--server-url", srv.URL, "--out-dir", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "nothing to speak") {
		t.Errorf("err = %v, want nothing to speak", err)
	}
}
