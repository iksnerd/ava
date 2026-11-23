package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iksnerd/ava/pkg/mlx"
	"github.com/iksnerd/ava/pkg/stt"
	"github.com/iksnerd/ava/pkg/stt/whisper"
)

// warmupBudget bounds the whole warm-up. It is a courtesy after a finished
// install, so it must never be what makes `ava setup` feel stuck.
const warmupBudget = 60 * time.Second

// warmUp reads the whisper model and, when an engine is already up, runs one
// short synthesis, so the first real dictation or speak does not pay for a
// cold disk cache or first-use setup. Output is discarded. It starts no
// engine, and a failure never fails setup: it prints one line and returns.
func warmUp(out io.Writer, skipEngine bool) {
	ctx, cancel := context.WithTimeout(context.Background(), warmupBudget)
	defer cancel()

	if err := warmTranscriber(); err != nil {
		warmupSkipped(out, err)
		return
	}
	if skipEngine {
		return
	}
	client := mlx.NewClient(setupEngineURL)
	if client.Check(ctx) != mlx.Ready {
		return // an engine that is down is not started for a warm-up
	}
	if _, err := client.Speak(ctx, mlx.SpeakOptions{Text: "ready", Voice: "af_heart", Speed: 1}); err != nil {
		warmupSkipped(out, err)
	}
}

// warmTranscriber runs the installed model over a short silent WAV. Silence
// yields no speech, which is the expected result and not an error.
func warmTranscriber() error {
	dir, err := os.MkdirTemp("", "ava-warmup-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	wav := filepath.Join(dir, "silence.wav")
	if err := os.WriteFile(wav, silentWAV(16000, time.Second), 0o600); err != nil {
		return err
	}
	t, err := newTranscriber("base")
	if err != nil {
		return err
	}
	if c, ok := t.(*whisper.Client); ok {
		c.Timeout = warmupBudget
	}
	_, err = t.Transcribe(stt.Options{AudioPath: wav, Language: "en"})
	return err
}

// silentWAV is 16-bit mono PCM silence, the format whisper-cli reads.
func silentWAV(rate int, d time.Duration) []byte {
	n := int(int64(rate) * int64(d) / int64(time.Second))
	data := n * 2
	b := make([]byte, 44+data)
	copy(b[0:], "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(36+data))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1) // PCM
	binary.LittleEndian.PutUint16(b[22:], 1) // mono
	binary.LittleEndian.PutUint32(b[24:], uint32(rate))
	binary.LittleEndian.PutUint32(b[28:], uint32(rate*2))
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(data))
	return b
}

// hintStaleEngine tells the user, after an update, that a running engine
// predates the binary just installed. It only looks: nothing is started or
// stopped, and a down or current engine prints nothing.
func hintStaleEngine(out io.Writer, engineURL string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if mlx.NewClient(engineURL).Check(ctx) == mlx.Stale {
		fmt.Fprintln(out, "   The running mlx-engine is older than this ava; "+
			"restart it with `ava engine stop && ava engine start`")
	}
}

// warmupSkipped prints the one line a failed warm-up costs: the first line of
// the error, since some carry a how-to-fix paragraph that setup does not need.
func warmupSkipped(out io.Writer, err error) {
	first, _, _ := strings.Cut(err.Error(), "\n")
	fmt.Fprintf(out, "   Warm-up skipped: %s\n", first)
}
