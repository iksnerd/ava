package speaker_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/iksnerd/ava/internal/speaker"
	"github.com/iksnerd/ava/internal/voiceconfig"
	"github.com/iksnerd/ava/pkg/mlx"
)

// staleEngine answers /health the way an engine left running across an
// upgrade does: alive, but not on this ava's protocol.
type staleEngine struct{}

func (staleEngine) Check(context.Context) mlx.State { return mlx.Stale }
func (staleEngine) Speak(context.Context, mlx.SpeakOptions) ([]byte, error) {
	return nil, errors.New("unreachable")
}
func (staleEngine) SpeakTimed(context.Context, mlx.SpeakOptions) (mlx.TimedSpeech, error) {
	return mlx.TimedSpeech{}, errors.New("unreachable")
}

// A caller outside the package, which is all a future public speaker has, must
// be able to tell a stale engine from any other failure without matching text.
func TestStaleEngineIsMatchableFromOutsideThePackage(t *testing.T) {
	s := &speaker.Speaker{
		Engine:   staleEngine{},
		Settings: voiceconfig.Defaults,
	}
	err := s.Speak(context.Background(), "hi", speaker.Options{Out: filepath.Join(t.TempDir(), "x.wav")})
	if !errors.Is(err, speaker.ErrStaleEngine) {
		t.Fatalf("Speak() error = %v, want errors.Is ErrStaleEngine", err)
	}
	want := "the running mlx-engine is older than this ava; restart it with `ava engine stop && ava engine start`"
	if speaker.ErrStaleEngine.Error() != want {
		t.Errorf("ErrStaleEngine message = %q, want %q", speaker.ErrStaleEngine.Error(), want)
	}
}

func TestStartLimitedErrorIsUsableFromOutsideThePackage(t *testing.T) {
	until := time.Date(2026, 1, 2, 15, 4, 0, 0, time.Local)
	wrapped := fmt.Errorf("cannot write: %w", &speaker.StartLimitedError{Until: until, Max: 3})

	var limited *speaker.StartLimitedError
	if !errors.As(wrapped, &limited) {
		t.Fatalf("errors.As failed on %v", wrapped)
	}
	if !limited.Until.Equal(until) || limited.Max != 3 {
		t.Errorf("fields = %v / %d, want %v / 3", limited.Until, limited.Max, until)
	}
	want := "mlx-engine failed to start 3 times in a row; not trying again until 15:04"
	if limited.Error() != want {
		t.Errorf("Error() = %q, want %q", limited.Error(), want)
	}
}
