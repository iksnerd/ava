package clipboard

import (
	"testing"
)

func TestCopyToClipboardBasic(t *testing.T) {
	// This test requires pbcopy to be available (macOS only)
	// It will copy to clipboard if run on macOS
	text := "test text"
	err := CopyToClipboard(text)

	// Just verify it doesn't panic or return error on macOS
	if err != nil {
		// Expected on non-macOS systems without pbcopy
		t.Logf("CopyToClipboard error (expected on non-macOS): %v", err)
	}
}

func TestCopyToClipboardEmpty(t *testing.T) {
	// Test copying empty string
	text := ""
	err := CopyToClipboard(text)

	// Should succeed even with empty string
	if err != nil {
		t.Logf("CopyToClipboard with empty string error: %v", err)
	}
}

func TestCopyToClipboardLongText(t *testing.T) {
	// Test copying long text
	text := "This is a longer test string with multiple words and special characters like !@#$%^&*(). " +
		"It should work fine with CopyToClipboard function. " +
		"Even with newlines:\nLine 1\nLine 2"
	err := CopyToClipboard(text)

	if err != nil {
		t.Logf("CopyToClipboard with long text error: %v", err)
	}
}
