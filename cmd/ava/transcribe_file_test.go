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

// transcribeWithFormat runs `transcribe` over a valid WAV with the CLI
// fixture whisper-cli, which writes one timed segment when asked for JSON.
func transcribeWithFormat(t *testing.T, args ...string) (string, error) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	model := filepath.Join(home, modelDirRel, baseModel)
	os.MkdirAll(filepath.Dir(model), 0755)
	os.WriteFile(model, []byte("placeholder"), 0644)
	testutil.PrependPath(t, "testdata/bin")

	clip := filepath.Join(t.TempDir(), "clip.wav")
	os.WriteFile(clip, append([]byte("RIFF\x00\x00\x00\x00WAVE"), make([]byte, 32)...), 0644)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"transcribe"}, append(args, clip)...))
	err := root.Execute()
	return out.String(), err
}

func TestTranscribeFormatSRTPrintsTimedCaptions(t *testing.T) {
	out, err := transcribeWithFormat(t, "--format", "srt")
	if err != nil {
		t.Fatalf("transcribe --format srt: %v", err)
	}
	want := "1\n00:00:00,000 --> 00:00:01,500\nHello there.\n"
	if strings.TrimSpace(out) != strings.TrimSpace(want) {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestTranscribeFormatWritesTheCaptionsToOutput(t *testing.T) {
	file := filepath.Join(t.TempDir(), "clip.vtt")
	if _, err := transcribeWithFormat(t, "--format", "vtt", "--output", file); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(file)
	if !strings.HasPrefix(string(got), "WEBVTT\n") || !strings.Contains(string(got), "00:00:00.000 --> 00:00:01.500") {
		t.Errorf("--output file = %q, want the WebVTT captions", got)
	}
}

func TestTranscribeRejectsAnUnknownFormat(t *testing.T) {
	_, err := transcribeWithFormat(t, "--format", "docx")
	if err == nil || !strings.Contains(err.Error(), "srt") {
		t.Errorf("err = %v, want one that lists the formats", err)
	}
}
