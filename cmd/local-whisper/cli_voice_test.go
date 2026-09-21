package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestVoiceCommandsResolve(t *testing.T) {
	root := newRootCmd()
	for _, name := range []string{"speak", "stop", "voices", "transcribe"} {
		c, _, err := root.Find([]string{name})
		if err != nil || c.Name() != name {
			t.Errorf("command %q not wired: %v", name, err)
		}
	}
}

func TestSpeakCommandFlags(t *testing.T) {
	c, _, _ := newRootCmd().Find([]string{"speak"})
	for _, f := range []string{"voice", "speed", "async", "server-url"} {
		if c.Flags().Lookup(f) == nil {
			t.Errorf("speak missing --%s", f)
		}
	}
}

func TestTranscribeCommandFlags(t *testing.T) {
	c, _, _ := newRootCmd().Find([]string{"transcribe"})
	for _, f := range []string{"lang", "engine", "model", "output"} {
		if c.Flags().Lookup(f) == nil {
			t.Errorf("transcribe missing --%s", f)
		}
	}
}

func TestReadSpeakTextJoinsArguments(t *testing.T) {
	got, err := readSpeakText([]string{"hello", "there"}, strings.NewReader(""))
	if err != nil {
		t.Fatalf("readSpeakText() error = %v", err)
	}
	if got != "hello there" {
		t.Errorf("text = %q, want %q", got, "hello there")
	}
}

// `some-command | local-whisper speak` is the reason this command exists at
// all — reading a build log or a diff aloud shouldn't need shell quoting.
func TestReadSpeakTextFallsBackToStdin(t *testing.T) {
	got, err := readSpeakText(nil, strings.NewReader("piped in\n"))
	if err != nil {
		t.Fatalf("readSpeakText() error = %v", err)
	}
	if got != "piped in" {
		t.Errorf("text = %q, want %q", got, "piped in")
	}
}

func TestReadSpeakTextRejectsNothingToSay(t *testing.T) {
	if _, err := readSpeakText(nil, strings.NewReader("   \n")); err == nil {
		t.Error("expected an error when neither arguments nor stdin carry text")
	}
}

// The CLI and the MCP list_voices tool must not drift into two different
// answers to the same question, so they share this formatter.
func TestFormatVoicesMarksTheCurrentVoice(t *testing.T) {
	got := formatVoices("bm_george")

	if !strings.Contains(got, "bm_george") || !strings.Contains(got, "British English, male") {
		t.Errorf("output missing the voice and its description:\n%s", got)
	}
	if !strings.Contains(got, "af_heart") {
		t.Errorf("output missing other voices:\n%s", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "bm_george") && !strings.Contains(line, "current") {
			t.Errorf("the current voice isn't marked on its own line: %q", line)
		}
	}
	if !strings.Contains(got, "af_heart,af_sky") {
		t.Errorf("output should mention voice blending:\n%s", got)
	}
}

// whisper-cli answers a missing file by printing its entire help screen —
// ~100 lines of flags for what is really "no such file". The command has to
// catch that before shelling out.
func TestTranscribeRejectsAMissingFileBeforeShellingOut(t *testing.T) {
	root := newRootCmd()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs([]string{"transcribe", filepath.Join(t.TempDir(), "absent.wav")})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for a file that doesn't exist")
	}
	if !strings.Contains(err.Error(), "absent.wav") {
		t.Errorf("error = %v, want it to name the file", err)
	}
	if strings.Contains(err.Error(), "--beam-size") || strings.Contains(out.String(), "--beam-size") {
		t.Errorf("whisper-cli's help leaked into the error:\n%v", err)
	}
}

func TestTranscribeRejectsADirectory(t *testing.T) {
	err := checkAudioFile(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "not a file") {
		t.Errorf("err = %v, want a not-a-file error for a directory", err)
	}
}

func TestA11yCommandWired(t *testing.T) {
	c, _, err := newRootCmd().Find([]string{"a11y"})
	if err != nil || c.Name() != "a11y" {
		t.Fatalf("a11y not wired: %v", err)
	}
	for _, f := range []string{"mode", "quiet", "voice", "speed"} {
		if c.Flags().Lookup(f) == nil {
			t.Errorf("a11y missing --%s", f)
		}
	}
}

func TestA11yPrintsAnnouncementsAndFindings(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader("uid=1_0 RootWebArea \"Acme\"\n  uid=1_1 heading \"Overview\" level=\"1\"\n  uid=1_2 button \"\"\n"))
	root.SetArgs([]string{"a11y", "--quiet"})

	if err := root.Execute(); err != nil {
		t.Fatalf("a11y --quiet: %v", err)
	}
	got := out.String()
	for _, want := range []string{"heading level 1, Overview", "button, unlabeled", "button with no accessible name"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}

func TestA11yRejectsInputThatIsNotASnapshot(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader("this is not an accessibility tree\n"))
	root.SetArgs([]string{"a11y", "--quiet"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "take_snapshot") {
		t.Errorf("err = %v, want it to name the tool that produces a snapshot", err)
	}
}

// Announce() falls back to reading for an unknown mode, which is right for
// an agent and wrong for a CLI: `--mode headins` should say so, not quietly
// read the entire page aloud.
func TestA11yRejectsAnUnknownMode(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader("uid=1_0 RootWebArea \"Acme\"\n"))
	root.SetArgs([]string{"a11y", "--quiet", "--mode", "headins"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for an unknown --mode")
	}
	if !strings.Contains(err.Error(), "headins") || !strings.Contains(err.Error(), "headings") {
		t.Errorf("error = %v, want it to quote the typo and list the valid modes", err)
	}
	if strings.Contains(out.String(), "web page, Acme") {
		t.Error("the page was announced anyway despite the bad mode")
	}
}

// Findings are page-wide, not mode-scoped, so a headings pass reports a link
// problem too. The header has to say that or it reads as a bug.
func TestA11yFindingsHeaderSaysItIsPageWide(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader("uid=1_0 RootWebArea \"Acme\"\n  uid=1_1 heading \"Guide\" level=\"2\"\n"))
	root.SetArgs([]string{"a11y", "--quiet", "--mode", "headings"})

	if err := root.Execute(); err != nil {
		t.Fatalf("a11y: %v", err)
	}
	if !strings.Contains(out.String(), "whole page") {
		t.Errorf("findings header doesn't say it covers the whole page:\n%s", out.String())
	}
}
