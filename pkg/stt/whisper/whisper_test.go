package whisper

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/iksnerd/ava/internal/testutil"
	"github.com/iksnerd/ava/pkg/stt"
)

const fixtureModelPath = "testdata/model.bin"

// fixtureClient is a client whose stub whisper-cli can read the WHISPER_*
// variables a test sets. whisper-cli itself is started with a clean environment,
// so the stubs get their data through the one door the tests control.
func fixtureClient() *Client {
	c := NewClient(fixtureModelPath)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "WHISPER_") {
			c.extraEnv = append(c.extraEnv, kv)
		}
	}
	return c
}

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
	if !strings.Contains(err.Error(), "ava setup-model") {
		t.Errorf("the error does not name the command that fixes it: %v", err)
	}
}

func TestTranscribeModelNotFoundNamesTheTinyModel(t *testing.T) {
	_, err := NewClient("/nonexistent/ggml-tiny.en.bin").Transcribe(stt.Options{})
	if err == nil || !strings.Contains(err.Error(), "setup-model --model tiny") {
		t.Errorf("a missing tiny model should point at `setup-model --model tiny`, got %v", err)
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

	client := fixtureClient()
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

func TestTranscribePassesBeamSize(t *testing.T) {
	for _, tc := range []struct {
		beam int
		want string
	}{
		{beam: 2, want: "beam=2"},
		// Unset means whisper-cli's own default, so -bs must not be passed.
		{beam: 0, want: "beam=\n"},
	} {
		testutil.PrependPath(t, "testdata/bin")
		dir := t.TempDir()
		audioPath := filepath.Join(dir, "audio.wav")
		os.WriteFile(audioPath, []byte("fixture audio"), 0644)
		logPath := filepath.Join(dir, "whisper.log")
		t.Setenv("WHISPER_LOG", logPath)

		if _, err := fixtureClient().Transcribe(stt.Options{AudioPath: audioPath, BeamSize: tc.beam}); err != nil {
			t.Fatalf("BeamSize %d: %v", tc.beam, err)
		}
		log, _ := os.ReadFile(logPath)
		if !strings.Contains(string(log), tc.want) {
			t.Errorf("BeamSize %d: whisper-cli invocation log = %q, want %q", tc.beam, log, tc.want)
		}
	}
}

func TestBuildArgsOmitsBeamSizeWhenUnset(t *testing.T) {
	for _, a := range buildArgs("m.bin", stt.Options{}) {
		if a == "-bs" {
			t.Fatal("-bs was passed with BeamSize unset; that overrides whisper-cli's own default")
		}
	}
	args := strings.Join(buildArgs("m.bin", stt.Options{BeamSize: 3}), " ")
	if !strings.Contains(args, "-bs 3") {
		t.Errorf("argv = %q, want -bs 3", args)
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

	client := fixtureClient()
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

	client := fixtureClient()
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

	client := fixtureClient()
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

	client := fixtureClient()
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

// whisper-cli ran with no timeout, so a hung process blocked its caller
// forever: the CLI, and an MCP transcribe call with the session waiting on it.
func TestTranscribeGivesUpOnAHungWhisperCli(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin-hang")
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "audio.wav")
	os.WriteFile(audioPath, []byte("fixture audio"), 0644)

	client := fixtureClient()
	client.Timeout = 200 * time.Millisecond

	start := time.Now()
	_, err := client.Transcribe(stt.Options{AudioPath: audioPath})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("err = %v, want a timeout", err)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("Transcribe returned after %s; the timeout did not stop whisper-cli", took)
	}
}

func TestNewClientHasATimeout(t *testing.T) {
	if NewClient("m.bin").Timeout <= 0 {
		t.Error("a default client has no timeout, so a hung whisper-cli blocks forever")
	}
}

// A caption format asks whisper-cli for its JSON, which carries the segment
// times, and renders the captions from that. Plain text keeps -nt and the
// stdout path it always had.
func TestTranscribeFormatsSegmentsAsCaptions(t *testing.T) {
	for format, want := range map[string]string{
		"srt": "1\n00:00:00,000 --> 00:00:02,500\nOnce upon a time.\n\n2\n00:00:02,500 --> 00:00:04,000\nThere was a lamp.\n",
		"vtt": "WEBVTT\n\n00:00:00.000 --> 00:00:02.500\nOnce upon a time.\n\n00:00:02.500 --> 00:00:04.000\nThere was a lamp.\n",
	} {
		t.Run(format, func(t *testing.T) {
			testutil.PrependPath(t, "testdata/bin")
			dir := t.TempDir()
			audio := filepath.Join(dir, "a.wav")
			os.WriteFile(audio, []byte("fixture audio"), 0644)
			logPath := filepath.Join(dir, "whisper.log")
			t.Setenv("WHISPER_LOG", logPath)

			got, err := fixtureClient().Transcribe(stt.Options{AudioPath: audio, Format: format})
			if err != nil {
				t.Fatalf("Transcribe(%s): %v", format, err)
			}
			if got != want {
				t.Errorf("%s =\n%q\nwant\n%q", format, got, want)
			}
			log, _ := os.ReadFile(logPath)
			if !strings.Contains(string(log), "oj=1") {
				t.Errorf("whisper-cli was not asked for JSON: %q", log)
			}
		})
	}
}

func TestTranscribeFormatJSONIsMillisecondOffsets(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin")
	audio := filepath.Join(t.TempDir(), "a.wav")
	os.WriteFile(audio, []byte("x"), 0644)

	got, err := fixtureClient().Transcribe(stt.Options{AudioPath: audio, Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"start_ms":0,"end_ms":2500,"text":"Once upon a time."},{"start_ms":2500,"end_ms":4000,"text":"There was a lamp."}]`
	if strings.TrimSpace(got) != want {
		t.Errorf("json = %s, want %s", got, want)
	}
}

func TestTranscribeTextFormatStillUsesNoTimestamps(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin")
	dir := t.TempDir()
	audio := filepath.Join(dir, "a.wav")
	os.WriteFile(audio, []byte("x"), 0644)
	logPath := filepath.Join(dir, "whisper.log")
	t.Setenv("WHISPER_LOG", logPath)

	for _, format := range []string{"", "text"} {
		if _, err := fixtureClient().Transcribe(stt.Options{AudioPath: audio, Format: format}); err != nil {
			t.Fatal(err)
		}
		log, _ := os.ReadFile(logPath)
		if !strings.Contains(string(log), "nt=1") || strings.Contains(string(log), "oj=1") {
			t.Errorf("format %q: log %q, want -nt and no -oj", format, log)
		}
	}
}

// A run that exits 0 and writes no JSON is a failure to report, not an empty
// transcript: it would read as "no speech" and send the user to their mic.
func TestTranscribeFormatFailsWhenWhisperWritesNoJSON(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin")
	t.Setenv("WHISPER_NO_JSON", "1")
	audio := filepath.Join(t.TempDir(), "a.wav")
	os.WriteFile(audio, []byte("x"), 0644)

	_, err := fixtureClient().Transcribe(stt.Options{AudioPath: audio, Format: "srt"})
	if err == nil || !strings.Contains(err.Error(), "timestamps") {
		t.Fatalf("err = %v, want one saying whisper-cli wrote no timestamps", err)
	}
}

func TestTranscribeFormatWithNoSpeechIsEmpty(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin")
	t.Setenv("WHISPER_FIXTURE_JSON", `{"transcription":[]}`)
	audio := filepath.Join(t.TempDir(), "a.wav")
	os.WriteFile(audio, []byte("x"), 0644)

	got, err := fixtureClient().Transcribe(stt.Options{AudioPath: audio, Format: "srt"})
	if err != nil || got != "" {
		t.Errorf("got %q, %v; want an empty result so the caller can say 'no speech'", got, err)
	}
}

// A whisper-cli that starts children and hangs: on a timeout the children must
// go with it, not be left running after the error is reported.
func TestATimeoutKillsWhisperCliAndItsChildren(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin-grandchild")
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	t.Setenv("WHISPER_CHILD_PIDFILE", pidFile)
	audio := filepath.Join(t.TempDir(), "a.wav")
	os.WriteFile(audio, []byte("x"), 0o644)

	client := fixtureClient()
	client.Timeout = 500 * time.Millisecond
	start := time.Now()
	if _, err := client.Transcribe(stt.Options{AudioPath: audio}); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want a timeout", err)
	}
	// Killing the group closes the output pipe at once. Killing only the
	// program leaves the child holding it, and Output() waits out WaitDelay (2s).
	if took := time.Since(start); took > 1500*time.Millisecond {
		t.Errorf("the timeout took %v to report: the child was still holding the output pipe", took)
	}

	var child int
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && child == 0 {
		if raw, err := os.ReadFile(pidFile); err == nil {
			child, _ = strconv.Atoi(strings.TrimSpace(string(raw)))
		}
		time.Sleep(10 * time.Millisecond)
	}
	if child == 0 {
		t.Fatal("the fixture never wrote its child's pid")
	}
	defer syscall.Kill(child, syscall.SIGKILL)
	for time.Now().Before(deadline.Add(2 * time.Second)) {
		if syscall.Kill(child, 0) != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("whisper-cli's child %d is still running after the timeout", child)
}

// A whisper-cli that finishes and leaves a child running: the call succeeds,
// and the child does not outlive it.
func TestAChildLeftRunningAfterASuccessfulRunIsReaped(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin-straggler")
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	t.Setenv("WHISPER_CHILD_PIDFILE", pidFile)
	audio := filepath.Join(t.TempDir(), "a.wav")
	os.WriteFile(audio, []byte("x"), 0o644)

	text, err := fixtureClient().Transcribe(stt.Options{AudioPath: audio})
	if err != nil || text != "a transcript" {
		t.Fatalf("text %q, err %v", text, err)
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	child, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	defer syscall.Kill(child, syscall.SIGKILL)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(child, 0) != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("whisper-cli's child %d is still running after the call returned", child)
}

// whisper-cli is started the way RFC 0002 section 4 has a host start a program:
// an empty environment plus PATH, HOME, LANG and TMPDIR. A token in the host's
// environment is not whisper-cli's business.
func TestWhisperCliGetsTheBaseEnvironmentOnly(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin-env")
	t.Setenv("HOME", "/srv/test-home")
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	os.Unsetenv("LANG")
	t.Setenv("API_TOKEN", "must-not-leak")
	t.Setenv("ANTHROPIC_API_KEY", "must-not-leak-either")
	audio := filepath.Join(tmp, "a.wav")
	os.WriteFile(audio, []byte("x"), 0o644)

	text, err := fixtureClient().Transcribe(stt.Options{AudioPath: audio})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"API_TOKEN=", "ANTHROPIC_API_KEY=", "LANG="} {
		if strings.Contains(text, name) {
			t.Errorf("whisper-cli saw %s in its environment:\n%s", name, text)
		}
	}
	for _, want := range []string{"HOME=/srv/test-home", "TMPDIR=" + tmp, "PATH="} {
		if !strings.Contains(text, want) {
			t.Errorf("whisper-cli's environment is missing %s:\n%s", want, text)
		}
	}
}

// Output over the cap ends the call at once and says why. The cap is lowered
// here so the input genuinely crosses it, and the fixture would write forever.
func TestOutputOverTheCapEndsTheRun(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin-flood")
	old := maxOutputBytes
	maxOutputBytes = 4096
	defer func() { maxOutputBytes = old }()
	audio := filepath.Join(t.TempDir(), "a.wav")
	os.WriteFile(audio, []byte("x"), 0o644)

	// A bound on the test itself: without the cap's kill the fixture never stops.
	client := fixtureClient()
	client.Timeout = 20 * time.Second

	start := time.Now()
	_, err := client.Transcribe(stt.Options{AudioPath: audio})
	if err == nil || !strings.Contains(err.Error(), "4096") || !strings.Contains(err.Error(), "output") {
		t.Fatalf("err = %v, want one naming the output cap of 4096 bytes", err)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("took %v: the run kept going after the cap instead of ending", took)
	}
}

// An error carries whisper-cli's stderr for diagnosis, but a program that writes
// a megabyte of it does not turn the error into a megabyte.
func TestAFailureKeepsOnlyTheStartOfStderr(t *testing.T) {
	testutil.PrependPath(t, "testdata/bin-noisy")
	audio := filepath.Join(t.TempDir(), "a.wav")
	os.WriteFile(audio, []byte("x"), 0o644)

	_, err := fixtureClient().Transcribe(stt.Options{AudioPath: audio})
	if err == nil || !strings.Contains(err.Error(), "eeee") {
		t.Fatalf("err = %v, want it to carry the start of stderr", err)
	}
	if len(err.Error()) > 20000 {
		t.Errorf("the error is %d bytes: stderr was not bounded", len(err.Error()))
	}
}
