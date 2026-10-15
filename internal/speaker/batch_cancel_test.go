package speaker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iksnerd/ava/internal/voiceconfig"
)

// A batch (--out-dir) cancelled while the engine is auto-starting must stop
// waiting for the start, which can take two minutes. This is the readiness step
// of the batch path, which TestCancelDuringAutoStartIsNotAFailedStart does not
// reach: if the batch passed no context to it, the start would run to the end.
func TestBatchCancelDuringAutoStartReturnsPromptlyAndWritesNothing(t *testing.T) {
	engine := &stubEngine{healthy: false}
	s, _ := newTestSpeaker(t, engine, voiceconfig.Defaults())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	entered := make(chan struct{})
	s.AutoStart = func(ctx context.Context) error {
		close(entered)
		<-ctx.Done() // a start that only ends when it is cancelled
		return ctx.Err()
	}
	dir := filepath.Join(t.TempDir(), "scenes")
	go func() { <-entered; cancel() }()

	err := within(t, 3*time.Second, "a batch cancelled during auto-start", func() error {
		return s.Speak(ctx, "one\ntwo", Options{OutDir: dir})
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Speak() error = %v, want context.Canceled", err)
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Errorf("the batch created %s after being cancelled", dir)
	}
}
