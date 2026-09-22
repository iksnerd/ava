package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeModel serves body from a local server and describes it the way
// whisperModels describes a real one, so downloadModel runs end to end
// without the network.
func fakeModel(t *testing.T, body string) (whisperModel, string, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	sum := sha256.Sum256([]byte(body))
	m := whisperModel{file: "ggml-fake.bin", size: int64(len(body)), sha256: hex.EncodeToString(sum[:])}
	return m, srv.URL + "/" + m.file, &hits
}

func TestDownloadModelInstallsAVerifiedFile(t *testing.T) {
	dir := t.TempDir()
	m, url, _ := fakeModel(t, "model bytes")

	if err := downloadModel(io.Discard, dir, m, url); err != nil {
		t.Fatalf("downloadModel: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, m.file))
	if err != nil || string(got) != "model bytes" {
		t.Fatalf("installed model = %q, %v; want the served bytes", got, err)
	}
}

func TestDownloadModelIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	m, url, hits := fakeModel(t, "model bytes")

	for i := 0; i < 2; i++ {
		if err := downloadModel(io.Discard, dir, m, url); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("the model was fetched %d times; a second run must skip it", n)
	}
}

// A truncated file from an interrupted curl is the realistic way a model ends
// up broken, and it has to be replaced rather than reported as installed.
func TestDownloadModelReplacesAWrongSizeFile(t *testing.T) {
	dir := t.TempDir()
	m, url, hits := fakeModel(t, "model bytes")
	os.WriteFile(filepath.Join(dir, m.file), []byte("mod"), 0644)

	if err := downloadModel(io.Discard, dir, m, url); err != nil {
		t.Fatalf("downloadModel: %v", err)
	}
	if hits.Load() != 1 {
		t.Error("a truncated model was left in place instead of being downloaded again")
	}
}

func TestDownloadModelRejectsAChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	m, url, _ := fakeModel(t, "model bytes")
	m.sha256 = strings.Repeat("0", 64)

	err := downloadModel(io.Discard, dir, m, url)
	if err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("err = %v, want a sha256 mismatch", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("a failed verification left files behind: %v", entries)
	}
}

func TestDownloadModelReportsAnHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	m := whisperModel{file: "ggml-fake.bin", size: 1, sha256: strings.Repeat("0", 64)}

	err := downloadModel(io.Discard, t.TempDir(), m, srv.URL+"/x")
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("err = %v, want the 404 named", err)
	}
}

// The URL must name a commit, not a branch: the hashes only mean something
// against the bytes of one fixed revision.
func TestModelURLsArePinned(t *testing.T) {
	commit := regexp.MustCompile(`^[0-9a-f]{40}$`)
	if !commit.MatchString(whisperModelRevision) {
		t.Fatalf("whisperModelRevision = %q, want a full commit sha", whisperModelRevision)
	}
	hash := regexp.MustCompile(`^[0-9a-f]{64}$`)
	for name, m := range whisperModels {
		if u := modelURL(m); !strings.Contains(u, "/resolve/"+whisperModelRevision+"/") {
			t.Errorf("%s: %s is not pinned to the revision", name, u)
		}
		if !hash.MatchString(m.sha256) || m.size <= 0 {
			t.Errorf("%s: sha256 %q / size %d is not a usable checksum", name, m.sha256, m.size)
		}
		if m.file != selectModelFile(name) {
			t.Errorf("%s: setup-model installs %s but transcription loads %s",
				name, m.file, selectModelFile(name))
		}
	}
}

func TestSetupModelIsRegisteredAndValidatesTheModel(t *testing.T) {
	root := newRootCmd()
	root.SetArgs([]string{"setup-model", "--model", "large"})
	root.SetOut(io.Discard)
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid model") {
		t.Fatalf("setup-model --model large: err = %v, want it rejected before any download", err)
	}
}
