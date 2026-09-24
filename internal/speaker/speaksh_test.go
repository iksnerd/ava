package speaker

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iksnerd/ava/internal/protocol"
	"github.com/iksnerd/ava/internal/testutil"
)

var speakScript = filepath.Join("..", "..", "scripts", "speak.sh")

// runSpeakSh runs scripts/speak.sh against stub curl, say and afplay, with
// every shared path (activity dir, playback lock, config, $TMPDIR) isolated,
// and waits for its backgrounded speak to finish. engineUp picks whether the
// stub server answers, and so whether the WAV or the `say` path runs.
func runSpeakSh(t *testing.T, engineUp bool) (tmp, audio string) {
	t.Helper()
	bin, tmp, work := t.TempDir(), t.TempDir(), t.TempDir()
	played := filepath.Join(work, "played")
	audioPath := filepath.Join(work, "audio-path")
	health := "exit 7"
	if engineUp {
		health = "exit 0"
	}
	stubs := map[string]string{
		// -o names the output file; /health has no -o.
		"curl": `out=""; url=""
while [ $# -gt 0 ]; do case "$1" in -o) out="$2"; shift ;; http*) url="$1" ;; esac; shift; done
case "$url" in */health) ` + health + ` ;; esac
[ -n "$out" ] && echo wav > "$out" && echo "$out" > ` + audioPath + `; exit 0`,
		"say":    `while [ $# -gt 0 ]; do [ "$1" = -o ] && echo aiff > "$2" && echo "$2" > ` + audioPath + `; shift; done`,
		"afplay": `touch ` + played,
	}
	for name, body := range stubs {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body+"\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	testutil.PrependPath(t, bin)
	config := filepath.Join(work, "config.json")
	if err := os.WriteFile(config, []byte(`{"engineAutoStart": false}`), 0644); err != nil {
		t.Fatal(err)
	}
	activity := filepath.Join(work, "active")
	t.Setenv("TMPDIR", tmp)
	t.Setenv("VOICE_CONFIG_FILE", config)
	t.Setenv(protocol.ActivityDirEnv, activity)
	t.Setenv(protocol.PlaybackLockEnv, filepath.Join(work, "playback.lock"))

	if out, err := exec.Command("bash", speakScript, "hello").CombinedOutput(); err != nil {
		t.Fatalf("speak.sh: %v\n%s", err, out)
	}
	// speak.sh returns at once and speaks in the background: done once the
	// player has run and the activity marker is gone.
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		entries, _ := os.ReadDir(activity)
		if _, err := os.Stat(played); err == nil && len(entries) == 0 {
			data, _ := os.ReadFile(audioPath)
			return tmp, strings.TrimSpace(string(data))
		}
		if time.Now().After(deadline) {
			t.Fatal("speak.sh never finished playing")
		}
	}
}

// Every speak.sh speak left an empty file behind: it appended .wav or .aiff
// to a name mktemp had already created, and removed only the suffixed one.
// It wrote them where `mktemp -t` chose, which on macOS ignores $TMPDIR, so
// they piled up in the per-user temp folder out of any test's sight.
func TestSpeakShLeavesNoTempFiles(t *testing.T) {
	for name, engineUp := range map[string]bool{"engine": true, "say fallback": false} {
		t.Run(name, func(t *testing.T) {
			tmp, audio := runSpeakSh(t, engineUp)
			if !strings.HasPrefix(audio, tmp+string(filepath.Separator)) {
				t.Fatalf("speak.sh wrote its audio to %q, outside $TMPDIR %q", audio, tmp)
			}
			entries, _ := os.ReadDir(tmp)
			for _, e := range entries {
				t.Errorf("left behind in $TMPDIR: %s", e.Name())
			}
		})
	}
}
