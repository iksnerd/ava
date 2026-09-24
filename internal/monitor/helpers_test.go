package monitor

import (
	"testing"
	"time"
)

func TestOrDefault(t *testing.T) {
	cases := []struct{ s, def, want string }{
		{"", "fallback", "fallback"},
		{"value", "fallback", "value"},
		{"", "", ""},
	}
	for _, tc := range cases {
		if got := orDefault(tc.s, tc.def); got != tc.want {
			t.Errorf("orDefault(%q, %q) = %q, want %q", tc.s, tc.def, got, tc.want)
		}
	}
}

func TestLanguageSuffix(t *testing.T) {
	cases := []struct{ language, want string }{
		{"", ""},
		{"en", ""},
		{"bg", ", language: bg"},
		{"es", ", language: es"},
	}
	for _, tc := range cases {
		if got := languageSuffix(tc.language); got != tc.want {
			t.Errorf("languageSuffix(%q) = %q, want %q", tc.language, got, tc.want)
		}
	}
}

func TestResolvePythonPath(t *testing.T) {
	cases := []struct {
		pythonPath, voxtralDir, want string
	}{
		{"", "voxtral", "voxtral/.venv/bin/python3"},
		{"/custom/python3", "voxtral", "/custom/python3"},
		{"", "/abs/voxtral", "/abs/voxtral/.venv/bin/python3"},
	}
	for _, tc := range cases {
		if got := resolvePythonPath(tc.pythonPath, tc.voxtralDir); got != tc.want {
			t.Errorf("resolvePythonPath(%q, %q) = %q, want %q", tc.pythonPath, tc.voxtralDir, got, tc.want)
		}
	}
}

func TestDefaultLogPath(t *testing.T) {
	fixedTime := time.Date(2026, 3, 5, 14, 30, 0, 0, time.UTC)
	got := defaultLogPath("/tmp/voice-input", fixedTime)
	want := "/tmp/voice-input/transcript-20260305-143000.txt"
	if got != want {
		t.Errorf("defaultLogPath() = %q, want %q", got, want)
	}
}
