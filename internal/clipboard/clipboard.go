package clipboard

import (
	"fmt"
	"os/exec"
)

// CopyToClipboard copies text to macOS clipboard using pbcopy
func CopyToClipboard(text string) error {
	cmd := exec.Command("pbcopy")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	if _, err := fmt.Fprint(stdin, text); err != nil {
		return err
	}

	if err := stdin.Close(); err != nil {
		return err
	}

	return cmd.Wait()
}

// PasteWithAppleScript auto-pastes using AppleScript (requires Accessibility permissions)
func PasteWithAppleScript() {
	script := `tell application "System Events" to keystroke "v" using {command down}`
	cmd := exec.Command("osascript", "-e", script)
	cmd.Run() // Ignore errors
}

// PlaySound plays a sound file asynchronously
func PlaySound(soundPath string, enabled bool) {
	if !enabled {
		return
	}
	cmd := exec.Command("afplay", soundPath)
	cmd.Run() // Ignore errors, just background play
}
