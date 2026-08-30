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

func TestValidateEngine(t *testing.T) {
	cases := []struct {
		engine  string
		wantErr bool
	}{
		{"whisper", false},
		{"voxtral", false},
		{"", true},
		{"WHISPER", true},
		{"gpt4", true},
	}
	for _, tc := range cases {
		err := validateEngine(tc.engine)
		if (err != nil) != tc.wantErr {
			t.Errorf("validateEngine(%q) error = %v, wantErr %v", tc.engine, err, tc.wantErr)
		}
	}
}

func TestValidateModel(t *testing.T) {
	cases := []struct {
		engine, model string
		wantErr       bool
	}{
		{"whisper", "base", false},
		{"whisper", "tiny", false},
		{"whisper", "large", true},
		{"whisper", "", true},
		// The model flag only constrains the whisper engine.
		{"voxtral", "large", false},
		{"voxtral", "", false},
	}
	for _, tc := range cases {
		err := validateModel(tc.engine, tc.model)
		if (err != nil) != tc.wantErr {
			t.Errorf("validateModel(%q, %q) error = %v, wantErr %v", tc.engine, tc.model, err, tc.wantErr)
		}
	}
}

func TestSelectModelFile(t *testing.T) {
	cases := []struct {
		modelName string
		want      string
	}{
		{"tiny", tinyModel},
		{"base", baseModel},
		{"", baseModel},
		{"anything-else", baseModel},
	}
	for _, tc := range cases {
		if got := selectModelFile(tc.modelName); got != tc.want {
			t.Errorf("selectModelFile(%q) = %q, want %q", tc.modelName, got, tc.want)
		}
	}
}

func TestLoadContextPrompt(t *testing.T) {
	dir := t.TempDir()
	contextFile := filepath.Join(dir, "context.txt")
	globalFile := filepath.Join(dir, "global.txt")
	writeFile(t, contextFile, "project context")
	writeFile(t, globalFile, "global context")
	missing := filepath.Join(dir, "missing.txt")

	t.Run("explicit context file wins", func(t *testing.T) {
		prompt, status := loadContextPrompt(contextFile, globalFile)
		if !strings.HasPrefix(prompt, "project context ") {
			t.Errorf("prompt = %q, want prefix %q", prompt, "project context ")
		}
		if !strings.Contains(status, contextFile) {
			t.Errorf("status = %q, want it to mention %q", status, contextFile)
		}
	})

	t.Run("falls back to global context", func(t *testing.T) {
		prompt, status := loadContextPrompt("", globalFile)
		if !strings.HasPrefix(prompt, "global context ") {
			t.Errorf("prompt = %q, want prefix %q", prompt, "global context ")
		}
		if !strings.Contains(status, "Global context loaded") {
			t.Errorf("status = %q, want it to mention global context", status)
		}
	})

	t.Run("missing explicit context file does not fall back", func(t *testing.T) {
		prompt, status := loadContextPrompt(missing, globalFile)
		if prompt != noContextPrompt {
			t.Errorf("prompt = %q, want bare %q (no silent fallback to global)", prompt, noContextPrompt)
		}
		if status != "" {
			t.Errorf("status = %q, want empty", status)
		}
	})

	t.Run("nothing configured", func(t *testing.T) {
		prompt, status := loadContextPrompt("", "")
		if prompt != noContextPrompt {
			t.Errorf("prompt = %q, want bare %q", prompt, noContextPrompt)
		}
		if status != "" {
			t.Errorf("status = %q, want empty", status)
		}
	})

	t.Run("missing global context is silently skipped", func(t *testing.T) {
		prompt, status := loadContextPrompt("", missing)
		if prompt != noContextPrompt || status != "" {
			t.Errorf("loadContextPrompt with missing global file = (%q, %q), want (%q, \"\")", prompt, status, noContextPrompt)
		}
	})
}

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

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
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
