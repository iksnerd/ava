package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"local-whisper/internal/testutil"
)

func TestCheckDependencies(t *testing.T) {
	t.Run("sox missing", func(t *testing.T) {
		testutil.SetPath(t, t.TempDir())
		err := checkDependencies("whisper", voxtralHealthURL)
		if err == nil || !strings.Contains(err.Error(), "sox is not installed") {
			t.Errorf("err = %v, want it to mention sox is not installed", err)
		}
	})

	t.Run("whisper-cli missing", func(t *testing.T) {
		soxOnly := t.TempDir()
		copyFixtureBin(t, soxOnly, "sox")
		testutil.SetPath(t, soxOnly)

		err := checkDependencies("whisper", voxtralHealthURL)
		if err == nil || !strings.Contains(err.Error(), "whisper-cli is not installed") {
			t.Errorf("err = %v, want it to mention whisper-cli is not installed", err)
		}
	})

	t.Run("whisper engine ok", func(t *testing.T) {
		testutil.SetPath(t, "testdata/bin")
		if err := checkDependencies("whisper", voxtralHealthURL); err != nil {
			t.Errorf("checkDependencies() = %v, want nil", err)
		}
	})

	t.Run("voxtral server up", func(t *testing.T) {
		testutil.SetPath(t, "testdata/bin")
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		if err := checkDependencies("voxtral", srv.URL+"/health"); err != nil {
			t.Errorf("checkDependencies() = %v, want nil", err)
		}
	})

	t.Run("voxtral server returns non-200", func(t *testing.T) {
		testutil.SetPath(t, "testdata/bin")
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer srv.Close()

		err := checkDependencies("voxtral", srv.URL+"/health")
		if err == nil || !strings.Contains(err.Error(), "voxtral server is not running") {
			t.Errorf("err = %v, want it to mention the voxtral server", err)
		}
	})

	t.Run("voxtral server unreachable", func(t *testing.T) {
		testutil.SetPath(t, "testdata/bin")
		srv := httptest.NewServer(nil)
		url := srv.URL
		srv.Close() // nothing is listening here anymore

		err := checkDependencies("voxtral", url+"/health")
		if err == nil || !strings.Contains(err.Error(), "voxtral server is not running") {
			t.Errorf("err = %v, want it to mention the voxtral server", err)
		}
	})
}

// copyFixtureBin copies a fixture executable from cmd/local-whisper/testdata/bin
// into dir, so a test can compose a PATH with only a subset of dependencies present.
func copyFixtureBin(t *testing.T, dir, name string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", "bin", name))
	if err != nil {
		t.Fatalf("read fixture bin %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), content, 0755); err != nil {
		t.Fatalf("write fixture bin %s: %v", name, err)
	}
}
