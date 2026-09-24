package main

import (
	"errors"
	"testing"
)

type fakeClipboard struct {
	copied  string
	pasted  bool
	copyErr error
}

func (f *fakeClipboard) ops() clipboardOps {
	return clipboardOps{
		copy:  func(text string) error { f.copied = text; return f.copyErr },
		paste: func() { f.pasted = true },
	}
}

// --no-paste used to skip the copy along with the paste, while the docs said
// it leaves the transcript on the clipboard and the CLI printed "Copied".
func TestNoPasteStillCopies(t *testing.T) {
	var cb fakeClipboard
	if err := cb.ops().deliver("hello", false); err != nil {
		t.Fatal(err)
	}
	if cb.copied != "hello" {
		t.Errorf("copied %q, want %q", cb.copied, "hello")
	}
	if cb.pasted {
		t.Error("pasted despite --no-paste")
	}
}

func TestDeliverCopiesThenPastes(t *testing.T) {
	var cb fakeClipboard
	if err := cb.ops().deliver("hello", true); err != nil {
		t.Fatal(err)
	}
	if cb.copied != "hello" || !cb.pasted {
		t.Errorf("copied %q, pasted %v; want both", cb.copied, cb.pasted)
	}
}

// Pasting after a failed copy would paste whatever was on the clipboard before.
func TestDeliverDoesNotPasteWhenTheCopyFails(t *testing.T) {
	cb := fakeClipboard{copyErr: errors.New("pbcopy failed")}
	if err := cb.ops().deliver("hello", true); err == nil {
		t.Error("expected the copy error back")
	}
	if cb.pasted {
		t.Error("pasted stale clipboard contents after the copy failed")
	}
}
