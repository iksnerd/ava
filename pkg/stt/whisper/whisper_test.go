package whisper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iksnerd/local-whisper/internal/testutil"
	"github.com/iksnerd/local-whisper/pkg/stt"
)

const fixtureModelPath = "testdata/model.bin"

func TestNewClient(t *testing.T) {
	modelPath := "/path/to/model.bin"
	client := NewClient(modelPath)

	if client.ModelPath != modelPath {
		t.Errorf("expected ModelPath %q, got %q", modelPath, client.ModelPath)
	}
}

func TestTranscribeModelNotFound(t *testing.T) {
	client := NewClient("/nonexistent/model.bin")

	_, err := client.Transcribe(stt.Options{
		AudioPath:     "/tmp/audio.wav",
		OutputPath:    "/tmp/output.txt",
		ContextPrompt: ".",
		Language:      "en",
	})
	if err == nil {
		t.Fatal("expected error when model not found, got nil")
	}
	if !strings.Contains(err.Error(), "whisper model not found") {
		t.Errorf("expected 'whisper model not found' in error, got %v", err)
	}
}

func TestTranscribeSuccess(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin")

	dir := t.TempDir()
	audioPath := filepath.Join(dir, "audio.wav")
	if err := os.WriteFile(audioPath, []byte("fixture audio"), 0644); err != nil {
		t.Fatalf("write fixture audio: %v", err)
	}
	outputPath := filepath.Join(dir, "prompt.txt")
	logPath := filepath.Join(dir, "whisper.log")
	t.Setenv("WHISPER_LOG", logPath)

	client := NewClient(fixtureModelPath)
	text, err := client.Transcribe(stt.Options{
		AudioPath:     audioPath,
		OutputPath:    outputPath,
		ContextPrompt: "some context",
		Language:      "bg",
	})
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if text != "fixture transcription" {
		t.Errorf("text = %q, want %q", text, "fixture transcription")
	}

	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read whisper-cli invocation log: %v", err)
	}
	for _, want := range []string{"audio=" + audioPath, "lang=bg", "prompt=some context", "model=" + fixtureModelPath} {
		if !strings.Contains(string(log), want) {
			t.Errorf("whisper-cli invocation log = %q, want it to contain %q", log, want)
		}
	}

	written, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(written) != "fixture transcription" {
		t.Errorf("output file content = %q, want the trimmed transcript with no extra newline", written)
	}
}

func TestTranscribeTrimsWhisperCliOutput(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin")

	dir := t.TempDir()
	audioPath := filepath.Join(dir, "audio.wav")
	os.WriteFile(audioPath, []byte("fixture audio"), 0644)

	// The real whisper-cli prints a leading newline+space before the
	// transcript with -nt; Transcribe must trim it rather than pass it
	// through.
	t.Setenv("WHISPER_FIXTURE_TEXT", "\n  padded transcript  \n")

	client := NewClient(fixtureModelPath)
	text, err := client.Transcribe(stt.Options{AudioPath: audioPath})
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if text != "padded transcript" {
		t.Errorf("text = %q, want trimmed %q", text, "padded transcript")
	}
}

func TestTranscribeNoOutputPathSkipsWrite(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin")

	dir := t.TempDir()
	audioPath := filepath.Join(dir, "audio.wav")
	os.WriteFile(audioPath, []byte("fixture audio"), 0644)

	client := NewClient(fixtureModelPath)
	text, err := client.Transcribe(stt.Options{AudioPath: audioPath})
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if text != "fixture transcription" {
		t.Errorf("text = %q, want %q", text, "fixture transcription")
	}
}

// A failure writing OutputPath is documented as non-fatal: the
// transcription itself already succeeded, so Transcribe should still
// return the text (matching pkg/mlxengine's behavior).
func TestTranscribeOutputWriteFailureIsNonFatal(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin")

	dir := t.TempDir()
	audioPath := filepath.Join(dir, "audio.wav")
	os.WriteFile(audioPath, []byte("fixture audio"), 0644)

	badOutputPath := filepath.Join(dir, "no-such-dir", "out.txt")

	client := NewClient(fixtureModelPath)
	text, err := client.Transcribe(stt.Options{AudioPath: audioPath, OutputPath: badOutputPath})
	if err != nil {
		t.Fatalf("Transcribe() error = %v, want nil despite the bad OutputPath", err)
	}
	if text != "fixture transcription" {
		t.Errorf("text = %q, want %q", text, "fixture transcription")
	}
}

func TestTranscribeCommandFailure(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin-fail")

	dir := t.TempDir()
	audioPath := filepath.Join(dir, "audio.wav")
	os.WriteFile(audioPath, []byte("fixture audio"), 0644)

	client := NewClient(fixtureModelPath)
	_, err := client.Transcribe(stt.Options{
		AudioPath:  audioPath,
		OutputPath: filepath.Join(dir, "prompt.txt"),
	})
	if err == nil {
		t.Fatal("expected an error when whisper-cli fails, got nil")
	}
	if !strings.Contains(err.Error(), "whisper-cli: fixture failure") {
		t.Errorf("err = %v, want it to include the captured stderr", err)
	}
}
