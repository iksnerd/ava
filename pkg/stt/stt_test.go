package stt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsNonSpeech(t *testing.T) {
	for _, s := range []string{"♪", " ♪ ♪ \n", "[BLANK_AUDIO]", "[MUSIC]\n♫", "[ Silence ]"} {
		if !IsNonSpeech(s) {
			t.Errorf("IsNonSpeech(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "hello", "[MUSIC] hello ♪", "(laughs)", "1 [x] 2"} {
		if IsNonSpeech(s) {
			t.Errorf("IsNonSpeech(%q) = true, want false", s)
		}
	}
}

func TestWriteOutputIfRequestedReturnsTheError(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "no-such-dir", "x.txt")
	err := WriteOutputIfRequested(Options{OutputPath: bad}, "hi")
	if err == nil || !strings.Contains(err.Error(), bad) {
		t.Errorf("err = %v, want one naming %s", err, bad)
	}
	good := filepath.Join(t.TempDir(), "x.txt")
	if err := WriteOutputIfRequested(Options{OutputPath: good}, "hi"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(good); string(b) != "hi" {
		t.Errorf("file = %q", b)
	}
	if err := WriteOutputIfRequested(Options{}, "hi"); err != nil {
		t.Errorf("no path: err = %v", err)
	}
}
