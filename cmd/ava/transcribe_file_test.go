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

// An --output the user asked for and that cannot be written is a failure,
// not a warning: a script that checks the exit code would otherwise believe
// the file exists. The transcript still reaches stdout first.
func TestTranscribeFailsWhenOutputCannotBeWritten(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "no-such-dir", "x.txt")
	out, err := transcribeWithFormat(t, "--format", "srt", "--output", bad)
	if err == nil || !strings.Contains(err.Error(), bad) {
		t.Fatalf("err = %v, want one naming %s", err, bad)
	}
	if !strings.Contains(out, "Hello there.") {
		t.Errorf("stdout = %q, want the transcript printed before the error", out)
	}
}

// transcribeWithStdout runs `transcribe` over a valid WAV where whisper-cli
// prints stdout in plain-text mode.
func transcribeWithStdout(t *testing.T, stdout string) (string, error) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	model := filepath.Join(home, modelDirRel, baseModel)
	os.MkdirAll(filepath.Dir(model), 0755)
	os.WriteFile(model, []byte("placeholder"), 0644)
	os.WriteFile(filepath.Join(home, "fixture-whisper-stdout"), []byte(stdout), 0644)
	testutil.PrependPath(t, "testdata/bin")

	clip := filepath.Join(t.TempDir(), "clip.wav")
	os.WriteFile(clip, append([]byte("RIFF\x00\x00\x00\x00WAVE"), make([]byte, 32)...), 0644)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"transcribe", clip})
	err := root.Execute()
	return out.String(), err
}

// A tone or silence makes whisper print only a marker. Text mode must say
// "no speech" like the caption formats do, not print the marker and exit 0.
func TestTranscribeTreatsOnlyMarkersAsNoSpeech(t *testing.T) {
	for _, marker := range []string{"♪\n", " [BLANK_AUDIO]\n", "[MUSIC]\n♪ ♪\n"} {
		out, err := transcribeWithStdout(t, marker)
		if err == nil || !strings.Contains(err.Error(), "no speech detected") {
			t.Errorf("%q: err = %v, want no speech detected", marker, err)
		}
		if strings.TrimSpace(out) != "" {
			t.Errorf("%q: stdout = %q, want nothing", marker, out)
		}
	}
}

func TestTranscribeKeepsSpeechThatCarriesAMarker(t *testing.T) {
	out, err := transcribeWithStdout(t, "[MUSIC] hello there ♪\n")
	if err != nil || !strings.Contains(out, "hello there") {
		t.Errorf("out = %q, err = %v, want the speech kept", out, err)
	}
}

// A file that is not audio gets the same message on every format, and never a
// temp path from the caption run.
func TestTranscribeNamesABadFileOnEveryFormat(t *testing.T) {
	for _, format := range []string{"text", "srt", "vtt", "json"} {
		err := transcribeBadFile(t, "--format", format)
		if err == nil || !strings.Contains(err.Error(), "too short") {
			t.Errorf("%s: err = %v, want the too-short-to-be-a-WAV message", format, err)
		}
		if err != nil && strings.Contains(err.Error(), "wrote no timestamps") {
			t.Errorf("%s: err = %v, leaked the engine's temp path", format, err)
		}
	}
}

func transcribeBadFile(t *testing.T, args ...string) error {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	model := filepath.Join(home, modelDirRel, baseModel)
	os.MkdirAll(filepath.Dir(model), 0755)
	os.WriteFile(model, []byte("placeholder"), 0644)
	testutil.PrependPath(t, "testdata/bin")

	clip := filepath.Join(t.TempDir(), "clip.wav")
	os.WriteFile(clip, []byte("0123456789"), 0644)

	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"transcribe"}, append(args, clip)...))
	return root.Execute()
}
