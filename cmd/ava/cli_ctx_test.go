package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mutedConfig points both config readers at a muted file, so a command that
// wrongly goes on to speak does so silently instead of out loud.
func mutedConfig(t *testing.T) {
	t.Helper()
	f := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(f, []byte(`{"muted": true}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VOICE_CONFIG_FILE", f)
	t.Setenv("VOICECONFIG_PATH", f)
	t.Setenv("TTS_VOLUME", "0")
}

func cancelledCtx() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// The commands run their speaker under the command's own context, so the
// context cobra was given (ExecuteContext) reaches the speak path. A command
// that used context.Background() would carry on, here silently.
func TestSpeakCommandsRunUnderTheCommandContext(t *testing.T) {
	mutedConfig(t)
	srv := fakeEngine(t, "RIFFwav")
	wav := filepath.Join(t.TempDir(), "x.wav")

	for name, tc := range map[string]struct {
		args  []string
		stdin string
	}{
		"speak":       {args: []string{"speak", "--server-url", srv.URL, "hello"}},
		"speak --out": {args: []string{"speak", "--server-url", srv.URL, "--out", wav, "hello"}},
		"a11y":        {args: []string{"a11y", "--server-url", srv.URL}, stdin: snapshotFixture},
	} {
		t.Run(name, func(t *testing.T) {
			root := newRootCmd()
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetIn(strings.NewReader(tc.stdin))
			root.SetArgs(tc.args)

			err := root.ExecuteContext(cancelledCtx())

			if !errors.Is(err, context.Canceled) {
				t.Fatalf("err = %v, want context.Canceled", err)
			}
			if _, statErr := os.Stat(wav); statErr == nil {
				t.Error("a cancelled speak --out wrote its file")
			}
		})
	}
}
