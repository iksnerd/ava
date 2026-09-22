package main

import (
	"bytes"
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
