package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iksnerd/ava/pkg/stt"
)

// The CLI rejects an unknown voice because the engine does not: it falls
// back to the macOS `say` voice and reports success. The MCP tools are the
// surface most likely to see an invented id (their callers are models), and
// they skipped the check.
func TestMcpSpeakRejectsAnUnknownVoice(t *testing.T) {
	s := &spy{}
	session := connect(t, s.deps())

	res := callTool(t, session, "speak", map[string]any{"text": "hi", "voice": "bf_emmma"})
	if !res.IsError {
		t.Errorf("speak with an unknown voice succeeded: %s", resultText(t, res))
	}
	if len(s.spoken) != 0 {
		t.Errorf("spoke %v with an unknown voice; it would have come out as `say`", s.spoken)
	}
}

func TestMcpSpeakAccessibilityTreeRejectsAnUnknownVoice(t *testing.T) {
	s := &spy{}
	session := connect(t, s.deps())

	res := callTool(t, session, "speak_accessibility_tree", map[string]any{"snapshot": snapshotFixture, "voice": "bf_emmma"})
	if !res.IsError {
		t.Errorf("speak_accessibility_tree with an unknown voice succeeded: %s", resultText(t, res))
	}
	if len(s.spoken) != 0 {
		t.Errorf("spoke %v with an unknown voice", s.spoken)
	}
}

func TestA11yRejectsAnUnknownVoiceBeforeAnnouncing(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(snapshotFixture))
	root.SetArgs([]string{"a11y", "--voice", "bf_emmma"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown voice") {
		t.Fatalf("a11y --voice bf_emmma: err = %v, want an unknown-voice error", err)
	}
	if strings.Contains(out.String(), "link, Home") {
		t.Error("a11y announced the page before rejecting the voice")
	}
}

// The CLI checks the file before whisper-cli sees it, because whisper-cli's
// answer to a missing file is its whole help screen.
func TestMcpTranscribeRejectsAMissingFile(t *testing.T) {
	s := &spy{transcribe: func(string, stt.Options) (string, error) {
		t.Error("the transcriber ran for a file that does not exist")
		return "", nil
	}}
	session := connect(t, s.deps())

	res := callTool(t, session, "transcribe", map[string]any{"audio_path": "/nonexistent/clip.wav"})
	if !res.IsError || !strings.Contains(resultText(t, res), "no such audio file") {
		t.Errorf("transcribe of a missing file = %q (error %v), want `no such audio file`", resultText(t, res), res.IsError)
	}
}

// `speak --out` is reachable from the MCP tool too, so an agent can narrate
// to a file without calling the engine's HTTP endpoint.
func TestMcpSpeakOutPassesThePathAndReportsTheFile(t *testing.T) {
	s := &spy{}
	session := connect(t, s.deps())

	res := callTool(t, session, "speak", map[string]any{"text": "scene one", "out": "/tmp/scene1.wav"})
	if res.IsError {
		t.Fatalf("speak with out failed: %s", resultText(t, res))
	}
	if len(s.speakOpts) != 1 || s.speakOpts[0].Out != "/tmp/scene1.wav" {
		t.Errorf("opts = %+v, want Out=/tmp/scene1.wav", s.speakOpts)
	}
	if !strings.Contains(resultText(t, res), "/tmp/scene1.wav") {
		t.Errorf("result = %q, want it to name the file", resultText(t, res))
	}
}

func TestMcpSpeakOutRejectsAsync(t *testing.T) {
	s := &spy{}
	session := connect(t, s.deps())

	res := callTool(t, session, "speak", map[string]any{"text": "hi", "out": "/tmp/a.wav", "async": true})
	if !res.IsError {
		t.Errorf("speak with out and async succeeded: %s", resultText(t, res))
	}
	if len(s.speakOpts) != 0 {
		t.Errorf("spoke with out and async: %+v", s.speakOpts)
	}
}

// transcribe's format is reachable from the MCP tool as well, with the same
// validation as the CLI.
func TestMcpTranscribePassesTheFormat(t *testing.T) {
	clip := filepath.Join(t.TempDir(), "clip.wav")
	os.WriteFile(clip, []byte("RIFF"), 0644)
	var got stt.Options
	s := &spy{transcribe: func(_ string, o stt.Options) (string, error) { got = o; return "1\nsrt body", nil }}
	session := connect(t, s.deps())

	res := callTool(t, session, "transcribe", map[string]any{"audio_path": clip, "format": "srt"})
	if res.IsError || got.Format != "srt" {
		t.Errorf("format = %q (error %v: %s), want srt", got.Format, res.IsError, resultText(t, res))
	}
}

func TestMcpTranscribeRejectsAnUnknownFormat(t *testing.T) {
	clip := filepath.Join(t.TempDir(), "clip.wav")
	os.WriteFile(clip, []byte("RIFF"), 0644)
	s := &spy{transcribe: func(string, stt.Options) (string, error) {
		t.Error("transcribed with an unknown format")
		return "", nil
	}}
	session := connect(t, s.deps())

	res := callTool(t, session, "transcribe", map[string]any{"audio_path": clip, "format": "docx"})
	if !res.IsError {
		t.Errorf("an unknown format succeeded: %s", resultText(t, res))
	}
}

func TestMcpSpeakPassesTimingsAndSplit(t *testing.T) {
	s := &spy{}
	session := connect(t, s.deps())

	res := callTool(t, session, "speak", map[string]any{"text": "hi", "out": "/tmp/a.wav", "timings": "/tmp/a.json", "split": "sentence"})
	if res.IsError {
		t.Fatalf("speak failed: %s", resultText(t, res))
	}
	o := s.speakOpts[0]
	if o.Timings != "/tmp/a.json" || o.SplitPattern == "" {
		t.Errorf("opts = %+v, want the timings path and the sentence pattern", o)
	}
}

func TestMcpSpeakTimingsNeedsOut(t *testing.T) {
	s := &spy{}
	session := connect(t, s.deps())

	res := callTool(t, session, "speak", map[string]any{"text": "hi", "timings": "/tmp/a.json"})
	if !res.IsError || len(s.speakOpts) != 0 {
		t.Errorf("timings without out: error=%v, spoke %d times", res.IsError, len(s.speakOpts))
	}
}

func TestMcpSpeakPassesOutDir(t *testing.T) {
	s := &spy{}
	session := connect(t, s.deps())

	res := callTool(t, session, "speak", map[string]any{"text": "one\ntwo", "out_dir": "/tmp/scenes", "split": "sentence"})
	if res.IsError {
		t.Fatalf("speak with out_dir failed: %s", resultText(t, res))
	}
	if o := s.speakOpts[0]; o.OutDir != "/tmp/scenes" || o.SplitPattern == "" || o.Out != "" {
		t.Errorf("opts = %+v, want OutDir and the sentence pattern", o)
	}
	if !strings.Contains(resultText(t, res), "/tmp/scenes") {
		t.Errorf("result = %q, want it to name the directory", resultText(t, res))
	}
}

func TestMcpSpeakOutDirExcludesOut(t *testing.T) {
	s := &spy{}
	session := connect(t, s.deps())

	res := callTool(t, session, "speak", map[string]any{"text": "hi", "out_dir": "/tmp/scenes", "out": "/tmp/a.wav"})
	if !res.IsError || len(s.speakOpts) != 0 {
		t.Errorf("out_dir with out: error=%v, spoke %d times", res.IsError, len(s.speakOpts))
	}
}

// An empty transcript means silence or a file whisper could not decode, and the
// CLI tells them apart. The MCP tool called both "no speech detected" and
// succeeded, so a model was told a corrupt file was a quiet room.
func TestMcpTranscribeNamesAFileThatIsNotAWav(t *testing.T) {
	clip := filepath.Join(t.TempDir(), "clip.wav")
	os.WriteFile(clip, append([]byte("FORM\x00\x00\x00\x00AIFF"), make([]byte, 32)...), 0644)
	s := &spy{transcribe: func(string, stt.Options) (string, error) { return "", nil }}
	session := connect(t, s.deps())

	res := callTool(t, session, "transcribe", map[string]any{"audio_path": clip})
	if !res.IsError || !strings.Contains(resultText(t, res), "not a WAV") {
		t.Errorf("error=%v, result %q; want an error saying the file is not a WAV", res.IsError, resultText(t, res))
	}
}

func TestMcpTranscribeStillReportsNoSpeechForARealWav(t *testing.T) {
	clip := filepath.Join(t.TempDir(), "clip.wav")
	os.WriteFile(clip, append([]byte("RIFF\x00\x00\x00\x00WAVE"), make([]byte, 32)...), 0644)
	s := &spy{transcribe: func(string, stt.Options) (string, error) { return "", nil }}
	session := connect(t, s.deps())

	res := callTool(t, session, "transcribe", map[string]any{"audio_path": clip})
	if res.IsError || !strings.Contains(resultText(t, res), "no speech detected") {
		t.Errorf("error=%v, result %q; want a plain no-speech result", res.IsError, resultText(t, res))
	}
}
