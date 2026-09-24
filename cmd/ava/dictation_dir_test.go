package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Dictation shares its temp root with ava monitor, whose call transcripts
// are kept there. Cleaning up used to be os.RemoveAll on the whole root, so
// dictating during a monitored call unlinked the call's transcript.
func TestDictationCleanupLeavesMonitorTranscriptsAlone(t *testing.T) {
	root := t.TempDir()
	transcript := filepath.Join(root, "transcript-2026-09-22.txt")
	if err := os.WriteFile(transcript, []byte("the call so far"), 0644); err != nil {
		t.Fatal(err)
	}

	dir, cleanup, err := newDictationDir(root)
	if err != nil {
		t.Fatalf("newDictationDir: %v", err)
	}
	if filepath.Dir(dir) != root {
		t.Errorf("dictation dir %s is not under %s", dir, root)
	}
	os.WriteFile(filepath.Join(dir, "prompt.wav"), []byte("audio"), 0644)
	cleanup()

	if _, err := os.Stat(transcript); err != nil {
		t.Errorf("cleanup removed ava monitor's transcript: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("cleanup left the dictation dir behind: %v", err)
	}
}

// Two dictations at once used to share one prompt.wav and overwrite it.
func TestConcurrentDictationsGetSeparateDirs(t *testing.T) {
	root := t.TempDir()
	a, cleanA, err := newDictationDir(root)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanA()
	b, cleanB, err := newDictationDir(root)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanB()
	if a == b {
		t.Errorf("two dictations share %s, so they overwrite each other's recording", a)
	}
}
