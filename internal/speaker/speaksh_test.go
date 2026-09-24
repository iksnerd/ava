package speaker

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/iksnerd/ava/internal/testutil"
)

// speak.sh (the Claude Code hooks, the menu bar's Test and Read Aloud) hands
// every speak to `ava speak`. It used to carry a second implementation of
// synthesis, the `say` fallback and the stop-aware player, and a stop fix
// applied to the Go one missed the bash one. Now there is one.

// Where a stub ava goes in the layout: a checkout's `make build` output, or
// beside scripts/ in the menu bar app's Resources, as build-app.sh bundles it.
const (
	noAva       = ""
	checkoutAva = "bin/ava"
	bundledAva  = "ava"
)

// speakShLayout copies scripts/*.sh into a throwaway checkout, so the real
// bin/ava a developer has built cannot stand in for the stub. ava names where
// to put a stub `ava` that records its arguments.
func speakShLayout(t *testing.T, ava string, muted bool) (script, argvFile string) {
	t.Helper()
	root := t.TempDir()
	if ava == bundledAva {
		root = filepath.Join(root, "Ava.app", "Contents", "Resources")
	}
	scripts := filepath.Join(root, "scripts")
	if err := os.MkdirAll(scripts, 0755); err != nil {
		t.Fatal(err)
	}
	sources, _ := filepath.Glob(filepath.Join("..", "..", "scripts", "*.sh"))
	sources = append(sources, filepath.Join("..", "..", "scripts", "voice-defaults.json"))
	for _, src := range sources {
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(scripts, filepath.Base(src)), data, 0755); err != nil {
			t.Fatal(err)
		}
	}

	argvFile = filepath.Join(t.TempDir(), "argv")
	if ava != noAva {
		path := filepath.Join(root, ava)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		stub := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\"; done > " + argvFile + "\n"
		if err := os.WriteFile(path, []byte(stub), 0755); err != nil {
			t.Fatal(err)
		}
	}

	config := filepath.Join(t.TempDir(), "config.json")
	body := `{"muted": false}`
	if muted {
		body = `{"muted": true}`
	}
	if err := os.WriteFile(config, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VOICE_CONFIG_FILE", config)
	// No real ava on PATH or in ~/.local/bin either.
	testutil.SetPath(t, "/usr/bin", "/bin")
	t.Setenv("HOME", t.TempDir())
	return filepath.Join(scripts, "speak.sh"), argvFile
}

func runSpeak(t *testing.T, script string, args ...string) (string, error) {
	t.Helper()
	out, err := exec.Command("bash", append([]string{script}, args...)...).CombinedOutput()
	return string(out), err
}

func handedOff(t *testing.T, argvFile string) []string {
	t.Helper()
	data, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatalf("speak.sh never called ava: %v", err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func TestSpeakShHandsOffToAvaSpeak(t *testing.T) {
	script, argv := speakShLayout(t, checkoutAva, false)
	if out, err := runSpeak(t, script, "build finished", "bf_emma"); err != nil {
		t.Fatalf("speak.sh: %v\n%s", err, out)
	}
	want := []string{"speak", "--async", "--voice", "bf_emma", "--", "build finished"}
	if got := handedOff(t, argv); !slices.Equal(got, want) {
		t.Errorf("ava called with %q, want %q", got, want)
	}
}

// The packaged menu bar app runs its bundled copy of scripts/, with the
// binary it shipped beside them rather than under bin/.
func TestSpeakShFindsTheAppBundlesAva(t *testing.T) {
	script, argv := speakShLayout(t, bundledAva, false)
	if out, err := runSpeak(t, script, "hello"); err != nil {
		t.Fatalf("speak.sh: %v\n%s", err, out)
	}
	handedOff(t, argv)
}

// A checkout's root holds whatever a bare `go build ./cmd/ava` left there
// (gitignored, often days stale). Only an app bundle keeps its binary beside
// scripts/; in a checkout, bin/ava wins.
func TestSpeakShIgnoresAStrayBuildInACheckoutRoot(t *testing.T) {
	script, argv := speakShLayout(t, checkoutAva, false)
	stray := filepath.Join(filepath.Dir(filepath.Dir(script)), "ava")
	if err := os.WriteFile(stray, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if out, err := runSpeak(t, script, "hello"); err != nil {
		t.Fatalf("speak.sh: %v\n%s", err, out)
	}
	handedOff(t, argv)
}

// Text starting with a dash must reach ava as text, not as a flag.
func TestSpeakShPassesDashedTextAsText(t *testing.T) {
	script, argv := speakShLayout(t, checkoutAva, false)
	if out, err := runSpeak(t, script, "-v is verbose"); err != nil {
		t.Fatalf("speak.sh: %v\n%s", err, out)
	}
	want := []string{"speak", "--async", "--", "-v is verbose"}
	if got := handedOff(t, argv); !slices.Equal(got, want) {
		t.Errorf("ava called with %q, want %q", got, want)
	}
}

// `ava speak` warns on stderr while muted; a hook firing on every turn
// should stay quiet instead.
func TestSpeakShDoesNothingWhileMuted(t *testing.T) {
	script, argv := speakShLayout(t, checkoutAva, true)
	if out, err := runSpeak(t, script, "hello"); err != nil {
		t.Fatalf("speak.sh: %v\n%s", err, out)
	}
	if _, err := os.Stat(argv); err == nil {
		t.Error("speak.sh called ava while muted")
	}
}

// Without a binary there is nothing to speak with, and a hook that exits 0
// having said nothing looks exactly like one that worked.
func TestSpeakShFailsLoudlyWithoutAva(t *testing.T) {
	script, _ := speakShLayout(t, noAva, false)
	out, err := runSpeak(t, script, "hello")
	if err == nil {
		t.Fatal("speak.sh exited 0 with no ava to speak with")
	}
	if !strings.Contains(out, "make build") || !strings.Contains(out, "install") {
		t.Errorf("speak.sh output %q: want it to say how to get an ava binary", out)
	}
}
