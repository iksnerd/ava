package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iksnerd/ava/internal/testutil"
)

// whisper-cli exits 0 with no output both for a silent recording and for a
// file it could not decode. The first is "no speech"; the second must name
// the file, or the user goes off to debug their microphone.
func transcribeWithEmptyWhisper(t *testing.T, content []byte) error {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	model := filepath.Join(home, modelDirRel, baseModel)
	os.MkdirAll(filepath.Dir(model), 0755)
	os.WriteFile(model, []byte("placeholder"), 0644)
	testutil.PrependPath(t, "testdata/bin") // a whisper-cli that prints nothing

	clip := filepath.Join(t.TempDir(), "clip.wav")
	os.WriteFile(clip, content, 0644)

	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"transcribe", clip})
	return root.Execute()
}

func TestTranscribeNamesAFileThatIsNotAWav(t *testing.T) {
	aiff := append([]byte("FORM\x00\x00\x00\x00AIFF"), make([]byte, 32)...)
	err := transcribeWithEmptyWhisper(t, aiff)
	if err == nil || !strings.Contains(err.Error(), "not a WAV file") || !strings.Contains(err.Error(), "FORM") {
		t.Errorf("err = %v, want it to say the file is not a WAV and show its FORM header", err)
	}
}

func TestTranscribeNamesAFileTooShortToBeAWav(t *testing.T) {
	err := transcribeWithEmptyWhisper(t, []byte("RIF"))
	if err == nil || !strings.Contains(err.Error(), "too short") {
		t.Errorf("err = %v, want it to say the file is too short to be a WAV", err)
	}
}

func TestTranscribeReportsNoSpeechForARealWav(t *testing.T) {
	wav := append([]byte("RIFF\x00\x00\x00\x00WAVE"), make([]byte, 32)...)
	err := transcribeWithEmptyWhisper(t, wav)
	if err == nil || !strings.Contains(err.Error(), "no speech detected") {
		t.Errorf("err = %v, want `no speech detected` for a valid but silent WAV", err)
	}
}
