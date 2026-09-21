package clipboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iksnerd/local-whisper/internal/testutil"
)

func TestCopyToClipboardWritesStdin(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin")

	cases := []struct {
		name string
		text string
	}{
		{"empty", ""},
		{"ascii", "hello clipboard"},
		{"unicode and emoji", "héllo 🎤 wörld"},
		{"multiline", "line one\nline two\nline three"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clipOut := filepath.Join(t.TempDir(), "clip.out")
			t.Setenv("CLIP_OUT", clipOut)

			if err := CopyToClipboard(tc.text); err != nil {
				t.Fatalf("CopyToClipboard(%q) error = %v", tc.text, err)
			}

			got, err := os.ReadFile(clipOut)
			if err != nil {
				t.Fatalf("read what pbcopy received: %v", err)
			}
			if string(got) != tc.text {
				t.Errorf("pbcopy received %q, want %q", got, tc.text)
			}
		})
	}
}

func TestCopyToClipboardCommandFailure(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin-fail")

	if err := CopyToClipboard("anything"); err == nil {
		t.Fatal("expected an error when pbcopy fails, got nil")
	}
}

func TestPasteWithAppleScriptInvokesOsascript(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin")

	logPath := filepath.Join(t.TempDir(), "osascript.log")
	t.Setenv("OSASCRIPT_LOG", logPath)

	PasteWithAppleScript()

	got, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read osascript invocation log: %v", err)
	}
	if !strings.Contains(string(got), `keystroke "v" using {command down}`) {
		t.Errorf("osascript args = %q, want the cmd-v keystroke script", got)
	}
}

func TestPlaySoundEnabled(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin")

	logPath := filepath.Join(t.TempDir(), "afplay.log")
	t.Setenv("AFPLAY_LOG", logPath)

	PlaySound("/System/Library/Sounds/Pop.aiff", true)

	got, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read afplay invocation log: %v", err)
	}
	if strings.TrimSpace(string(got)) != "/System/Library/Sounds/Pop.aiff" {
		t.Errorf("afplay was called with %q, want the Pop.aiff path", got)
	}
}

func TestPlaySoundDisabled(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin")

	logPath := filepath.Join(t.TempDir(), "afplay.log")
	t.Setenv("AFPLAY_LOG", logPath)

	PlaySound("/System/Library/Sounds/Pop.aiff", false)

	if _, err := os.Stat(logPath); err == nil {
		t.Error("expected afplay not to be invoked when enabled is false")
	}
}
